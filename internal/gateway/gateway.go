package gateway

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

type RecordEvent func(event string, record model.RequestRecord)

type Server struct {
	upstream       model.Upstream
	dialer         *UpstreamDialer
	routes         []model.RouteRule
	allowedHosts   []string
	hostFilter     string
	direct         *net.Dialer
	resolveProcess func(localPort int) string
	interfaceL     net.Listener
	kernelL        net.Listener
	interfaceS     *http.Server
	kernelS        *http.Server
	interfaceP     int
	kernelP        int
	onRecord       RecordEvent
	startedAt      time.Time
	closeOnce      sync.Once
}

type Options struct {
	Upstream            model.Upstream
	Host                string
	PreferredPort       int
	PreferredKernelPort int
	Routes              []model.RouteRule
	AllowedHosts        []string
	HostFilterMode      string
	ResolveProcess      func(localPort int) string
	OnRecord            RecordEvent
}

func Start(ctx context.Context, opts Options) (*Server, error) {
	dialer := NewUpstreamDialer(opts.Upstream)
	interfaceL, err := listen(opts.Host, opts.PreferredPort)
	if err != nil {
		return nil, fmt.Errorf("start Lane interface listener: %w", err)
	}
	kernelL, err := listen(opts.Host, opts.PreferredKernelPort)
	if err != nil {
		interfaceL.Close()
		return nil, fmt.Errorf("start Lane kernel listener: %w", err)
	}
	s := &Server{
		upstream:       opts.Upstream,
		dialer:         dialer,
		routes:         opts.Routes,
		allowedHosts:   opts.AllowedHosts,
		hostFilter:     opts.HostFilterMode,
		resolveProcess: opts.ResolveProcess,
		direct:         &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second},
		interfaceL:     interfaceL,
		kernelL:        kernelL,
		interfaceP:     portOf(interfaceL.Addr()),
		kernelP:        portOf(kernelL.Addr()),
		onRecord:       opts.OnRecord,
		startedAt:      time.Now(),
	}
	s.interfaceS = &http.Server{
		Handler:           s.handler(model.SourceInterface),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	s.kernelS = &http.Server{
		Handler:           s.handler(model.SourceKernel),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		_ = s.interfaceS.Serve(interfaceL)
	}()
	go func() {
		_ = s.kernelS.Serve(kernelL)
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		s.Close(shutdownCtx)
	}()
	return s, nil
}

func (s *Server) InterfacePort() int   { return s.interfaceP }
func (s *Server) KernelPort() int      { return s.kernelP }
func (s *Server) StartedAt() time.Time { return s.startedAt }

func (s *Server) InterfaceURL() string {
	return "http://127.0.0.1:" + fmt.Sprint(s.interfaceP)
}

func (s *Server) KernelURL() string {
	return "http://127.0.0.1:" + fmt.Sprint(s.kernelP)
}

func (s *Server) Close(ctx context.Context) error {
	var err error
	s.closeOnce.Do(func() {
		_ = s.interfaceS.Shutdown(ctx)
		_ = s.kernelS.Shutdown(ctx)
		_ = s.interfaceL.Close()
		_ = s.kernelL.Close()
	})
	return err
}

func (s *Server) handler(source string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			s.handleConnect(source, w, r)
			return
		}
		if r.URL.Host == "" {
			s.handleStatusPage(w, r)
			return
		}
		s.handleHTTP(source, w, r)
	})
}

func matchPattern(pattern string, host string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	host = strings.ToLower(host)
	if pattern == "*" {
		return true
	}
	if suffix, ok := strings.CutPrefix(pattern, "*."); ok {
		return host == suffix || strings.HasSuffix(host, "."+suffix)
	}
	return pattern == host
}

func (s *Server) routeAction(host string) string {
	for _, rule := range s.routes {
		if matchPattern(rule.Pattern, host) {
			return rule.Action
		}
	}
	return model.RouteProxy
}

func (s *Server) hostAllowed(host string) bool {
	if s.hostFilter != model.HostFilterAllowlist {
		return true
	}
	for _, pattern := range s.allowedHosts {
		if matchPattern(pattern, host) {
			return true
		}
	}
	return false
}

func (s *Server) dialTarget(ctx context.Context, host string, target string) (net.Conn, string, error) {
	if s.routeAction(host) == model.RouteDirect {
		conn, err := s.direct.DialContext(ctx, "tcp", target)
		return conn, "DIRECT", err
	}
	conn, err := s.dialer.Dial(ctx, target)
	return conn, upstreamLabel(s.upstream), err
}

func (s *Server) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, fmt.Sprintf(
			`{"ok":true,"interfacePort":%d,"kernelPort":%d,"upstream":%q,"startedAt":%q}`,
			s.interfaceP, s.kernelP, upstreamLabel(s.upstream), s.startedAt.Format(time.RFC3339),
		))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, statusPageHTML(s.interfaceP, s.kernelP, upstreamLabel(s.upstream)))
}

func statusPageHTML(interfacePort int, kernelPort int, upstream string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Codex Lane 本地入口</title>
<style>
:root { color-scheme: light; }
body { margin: 0; min-height: 100vh; display: grid; place-items: center;
  font-family: "Segoe UI", "Microsoft YaHei", system-ui, sans-serif;
  background: #f4f4f6; color: #17181c; }
main { width: min(560px, calc(100vw - 48px)); background: #fff; border-radius: 16px;
  padding: 32px; box-shadow: 0 12px 32px rgba(16, 18, 27, .08); }
h1 { margin: 0 0 8px; font-size: 22px; }
p { margin: 0 0 18px; color: #5b5f6b; line-height: 1.6; }
dl { display: grid; grid-template-columns: auto 1fr; gap: 10px 16px; margin: 0 0 22px;
  padding: 16px; background: #f7f7f9; border-radius: 12px; font-size: 14px; }
dt { color: #7a7e8a; }
dd { margin: 0; font-family: Consolas, monospace; }
small { color: #8a8e99; }
</style>
</head>
<body>
<main>
  <h1>Codex Lane 本地入口</h1>
  <p>这个端口是给 Codex 用的 HTTP 代理，不是网页控制台。浏览器直连会显示本页，属于正常现象。</p>
  <dl>
    <dt>UI 入口</dt><dd>127.0.0.1:%d</dd>
    <dt>内核入口</dt><dd>127.0.0.1:%d</dd>
    <dt>上游代理</dt><dd>%s</dd>
  </dl>
  <small>控制台请在 Codex Lane 窗口里操作。本页不读取、不转发任何请求。</small>
</main>
</body>
</html>`, interfacePort, kernelPort, upstream)
}

func (s *Server) handleConnect(source string, w http.ResponseWriter, r *http.Request) {
	host, target, port, err := ParseTarget(r.Host, 443)
	started := time.Now()
	id := newRequestID()
	record := model.RequestRecord{
		ID:        id,
		Time:      started.Format("15:04:05.000"),
		Source:    source,
		Method:    http.MethodConnect,
		Host:      host,
		Port:      port,
		Scheme:    "https",
		Upstream:  upstreamLabel(s.upstream),
		StartedAt: started.Format(time.RFC3339Nano),
	}
	if !s.hostAllowed(host) {
		record.Status = http.StatusForbidden
		record.Result = "blocked"
		record.Error = "host is not in the Lane allowlist"
		record.LatencyMs = time.Since(started).Milliseconds()
		record.DurationMs = record.LatencyMs
		record.FinishedAt = time.Now().Format(time.RFC3339Nano)
		s.emitFinished(record)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "Codex Lane: host is not in the allowlist")
		return
	}
	s.emit("request:new", record)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	upstreamConn, dialLabel, err := s.dialTarget(ctx, host, target)
	record.Upstream = dialLabel
	if err != nil {
		record.Status = http.StatusBadGateway
		record.Result = "failed"
		record.Error = err.Error()
		record.LatencyMs = time.Since(started).Milliseconds()
		record.DurationMs = time.Since(started).Milliseconds()
		record.FinishedAt = time.Now().Format(time.RFC3339Nano)
		s.emitFinished(record)
		http.Error(w, "Codex Lane upstream: "+err.Error(), http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstreamConn.Close()
		record.Status = http.StatusInternalServerError
		record.Result = "failed"
		record.Error = "HTTP hijacking is unavailable"
		record.LatencyMs = time.Since(started).Milliseconds()
		record.FinishedAt = time.Now().Format(time.RFC3339Nano)
		s.emitFinished(record)
		http.Error(w, record.Error, http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		upstreamConn.Close()
		record.Status = http.StatusInternalServerError
		record.Result = "failed"
		record.Error = err.Error()
		record.LatencyMs = time.Since(started).Milliseconds()
		record.FinishedAt = time.Now().Format(time.RFC3339Nano)
		s.emitFinished(record)
		return
	}
	if _, err := io.WriteString(clientConn, "HTTP/1.1 200 Connection Established\r\nProxy-Agent: CodexLane/0.1\r\n\r\n"); err != nil {
		clientConn.Close()
		upstreamConn.Close()
		record.Status = http.StatusBadGateway
		record.Result = "failed"
		record.Error = err.Error()
		record.LatencyMs = time.Since(started).Milliseconds()
		record.FinishedAt = time.Now().Format(time.RFC3339Nano)
		s.emitFinished(record)
		return
	}
	if s.resolveProcess != nil {
		if addr, ok := clientConn.RemoteAddr().(*net.TCPAddr); ok {
			record.Process = s.resolveProcess(addr.Port)
		}
	}
	record.Status = http.StatusOK
	record.Result = "ok"
	record.LatencyMs = time.Since(started).Milliseconds()
	s.emit("request:update", record)

	bytesUp, bytesDown := tunnel(clientConn, upstreamConn)
	record.BytesUp = bytesUp
	record.BytesDown = bytesDown
	record.DurationMs = time.Since(started).Milliseconds()
	record.FinishedAt = time.Now().Format(time.RFC3339Nano)
	s.emitFinished(record)
}

func (s *Server) handleHTTP(source string, w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	host := r.URL.Hostname()
	port := 80
	if r.URL.Port() != "" {
		if parsed, err := net.LookupPort("tcp", r.URL.Port()); err == nil {
			port = parsed
		}
	} else if r.URL.Scheme == "https" {
		port = 443
	}
	action := s.routeAction(host)
	dialLabel := upstreamLabel(s.upstream)
	if action == model.RouteDirect {
		dialLabel = "DIRECT"
	}
	record := model.RequestRecord{
		ID:        newRequestID(),
		Time:      started.Format("15:04:05.000"),
		Source:    source,
		Method:    r.Method,
		Host:      host,
		Port:      port,
		Scheme:    r.URL.Scheme,
		Upstream:  dialLabel,
		StartedAt: started.Format(time.RFC3339Nano),
	}
	if !s.hostAllowed(host) {
		record.Status = http.StatusForbidden
		record.Result = "blocked"
		record.Error = "host is not in the Lane allowlist"
		record.LatencyMs = time.Since(started).Milliseconds()
		record.DurationMs = record.LatencyMs
		record.FinishedAt = time.Now().Format(time.RFC3339Nano)
		s.emitFinished(record)
		http.Error(w, "Codex Lane: host is not in the allowlist", http.StatusForbidden)
		return
	}
	s.emit("request:new", record)

	var bytesUp, bytesDown atomic.Int64
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.RequestURI = ""
			if req.URL.Scheme == "" {
				req.URL.Scheme = "http"
			}
			removeHopByHop(req.Header)
		},
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				conn, _, err := s.dialTarget(ctx, host, address)
				if err != nil {
					return nil, err
				}
				return &countingConn{Conn: conn, read: &bytesDown, write: &bytesUp}, nil
			},
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       60 * time.Second,
		},
		ModifyResponse: func(response *http.Response) error {
			record.Status = response.StatusCode
			if response.StatusCode >= 200 && response.StatusCode < 400 {
				record.Result = "ok"
			} else {
				record.Result = "http"
			}
			s.emit("request:update", record)
			return nil
		},
		ErrorHandler: func(writer http.ResponseWriter, request *http.Request, err error) {
			record.Status = http.StatusBadGateway
			record.Result = "failed"
			record.Error = err.Error()
			record.LatencyMs = time.Since(started).Milliseconds()
			http.Error(writer, "Codex Lane upstream: "+err.Error(), http.StatusBadGateway)
		},
		FlushInterval: -1,
	}
	if s.resolveProcess != nil && r.RemoteAddr != "" {
		if _, portText, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			if port, err := net.LookupPort("tcp", portText); err == nil {
				record.Process = s.resolveProcess(port)
			}
		}
	}
	proxy.ServeHTTP(w, r)
	record.BytesUp = bytesUp.Load()
	record.BytesDown = bytesDown.Load()
	record.DurationMs = time.Since(started).Milliseconds()
	record.FinishedAt = time.Now().Format(time.RFC3339Nano)
	s.emitFinished(record)
}

func (s *Server) emit(event string, record model.RequestRecord) {
	if s.onRecord != nil {
		s.onRecord(event, record)
	}
}

func (s *Server) emitFinished(record model.RequestRecord) {
	s.emit("request:finished", record)
}

func tunnel(client net.Conn, upstream net.Conn) (int64, int64) {
	defer client.Close()
	defer upstream.Close()
	var bytesUp, bytesDown atomic.Int64
	done := make(chan struct{}, 2)
	go func() {
		n, _ := io.Copy(upstream, client)
		bytesUp.Store(n)
		if tcp, ok := upstream.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		n, _ := io.Copy(client, upstream)
		bytesDown.Store(n)
		if tcp, ok := client.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- struct{}{}
	}()
	<-done
	_ = client.SetDeadline(time.Now())
	_ = upstream.SetDeadline(time.Now())
	<-done
	return bytesUp.Load(), bytesDown.Load()
}

type countingConn struct {
	net.Conn
	read  *atomic.Int64
	write *atomic.Int64
}

func (c *countingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.read.Add(int64(n))
	return n, err
}

func (c *countingConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.write.Add(int64(n))
	return n, err
}

func listen(host string, port int) (net.Listener, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	return net.Listen("tcp", net.JoinHostPort(host, fmt.Sprint(port)))
}

func portOf(addr net.Addr) int {
	if tcp, ok := addr.(*net.TCPAddr); ok {
		return tcp.Port
	}
	return 0
}

func upstreamLabel(upstream model.Upstream) string {
	return strings.ToUpper(upstream.Protocol) + " · " + net.JoinHostPort(upstream.Host, fmt.Sprint(upstream.Port))
}

func newRequestID() string {
	var data [6]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return "req_" + hex.EncodeToString(data[:])
}

func removeHopByHop(header http.Header) {
	for _, key := range []string{
		"Proxy-Connection",
		"Proxy-Authorization",
		"Keep-Alive",
		"TE",
		"Trailer",
	} {
		header.Del(key)
	}
}

func CheckTCPService(ctx context.Context, address string) (time.Duration, error) {
	started := time.Now()
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return time.Since(started), err
	}
	_ = conn.Close()
	return time.Since(started), nil
}

func CheckUpstreamTunnel(ctx context.Context, upstream model.Upstream, target string) (time.Duration, error) {
	started := time.Now()
	dialer := NewUpstreamDialer(upstream)
	conn, err := dialer.Dial(ctx, target)
	if err != nil {
		return time.Since(started), err
	}
	_ = conn.Close()
	return time.Since(started), nil
}

// CheckHostTLS verifies that a host is reachable through the upstream proxy by
// completing a TLS handshake and, when possible, reading one HTTP response.
// Any HTTP status counts as reachable; only transport failures are errors.
func CheckHostTLS(ctx context.Context, upstream model.Upstream, host string) (status int, version string, latency time.Duration, err error) {
	started := time.Now()
	dialer := NewUpstreamDialer(upstream)
	return checkHostTLS(ctx, dialer.Dial, host, started)
}

// CheckHostTLSViaProxy runs the same check through a plain HTTP proxy URL,
// which is how the app verifies the full Codex Lane path.
func CheckHostTLSViaProxy(ctx context.Context, proxyURL, host string) (status int, version string, latency time.Duration, err error) {
	parsed, parseErr := url.Parse(proxyURL)
	if parseErr != nil {
		return 0, "", 0, parseErr
	}
	proxyHost, proxyPortText, splitErr := net.SplitHostPort(parsed.Host)
	if splitErr != nil {
		return 0, "", 0, splitErr
	}
	proxyPort, convErr := strconv.Atoi(proxyPortText)
	if convErr != nil {
		return 0, "", 0, convErr
	}
	dialer := NewUpstreamDialer(model.Upstream{
		Protocol: model.ProtocolHTTP,
		Host:     proxyHost,
		Port:     proxyPort,
	})
	return checkHostTLS(ctx, dialer.Dial, host, time.Now())
}

func checkHostTLS(ctx context.Context, dial func(context.Context, string) (net.Conn, error), host string, started time.Time) (status int, version string, latency time.Duration, err error) {
	conn, err := dial(ctx, net.JoinHostPort(host, "443"))
	if err != nil {
		return 0, "", time.Since(started), err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	})
	if handshakeErr := tlsConn.HandshakeContext(ctx); handshakeErr != nil {
		return 0, "", time.Since(started), fmt.Errorf("TLS 握手失败: %w", handshakeErr)
	}
	version = TLSVersionName(tlsConn.ConnectionState().Version)
	if _, writeErr := io.WriteString(tlsConn,
		"HEAD / HTTP/1.1\r\nHost: "+host+"\r\nUser-Agent: CodexLane/0.1\r\nAccept: */*\r\nConnection: close\r\n\r\n"); writeErr != nil {
		return 0, version, time.Since(started), nil
	}
	response, readErr := http.ReadResponse(bufio.NewReader(tlsConn), nil)
	if readErr != nil {
		return 0, version, time.Since(started), nil
	}
	defer response.Body.Close()
	return response.StatusCode, version, time.Since(started), nil
}

func TLSVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS10:
		return "TLS 1.0"
	default:
		return "TLS"
	}
}

func HTTPThroughProxy(ctx context.Context, proxyURL string, target string) (int, time.Duration, error) {
	status, _, duration, err := HTTPBodyThroughProxy(ctx, proxyURL, target)
	return status, duration, err
}

func HTTPBodyThroughProxy(ctx context.Context, proxyURL string, target string) (int, []byte, time.Duration, error) {
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return 0, nil, 0, err
	}
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:               http.ProxyURL(parsed),
			ForceAttemptHTTP2:   true,
			TLSHandshakeTimeout: 15 * time.Second,
		},
		Timeout: 25 * time.Second,
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, nil, 0, err
	}
	request.Header.Set("User-Agent", "CodexLane/0.1 connectivity-test")
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, time.Since(started), err
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 128*1024))
	if readErr != nil {
		return response.StatusCode, body, time.Since(started), readErr
	}
	return response.StatusCode, body, time.Since(started), nil
}

var ErrNoLane = errors.New("Lane gateway is not running")
