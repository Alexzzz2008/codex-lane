package gateway

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

type fakeUpstream struct {
	listener net.Listener
	conns    atomic.Int32
}

func startFakeUpstream(t *testing.T) *fakeUpstream {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake upstream: %v", err)
	}
	upstream := &fakeUpstream{listener: listener}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			upstream.conns.Add(1)
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				request, err := http.ReadRequest(bufio.NewReader(conn))
				if err != nil {
					return
				}
				if request.Method == http.MethodConnect {
					_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
					for {
						tunnelled, err := http.ReadRequest(bufio.NewReader(conn))
						if err != nil {
							return
						}
						_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 13\r\n\r\nfrom-upstream")
						_ = tunnelled.Body.Close()
					}
				}
				_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 13\r\n\r\nfrom-upstream")
			}()
		}
	}()
	t.Cleanup(func() { listener.Close() })
	return upstream
}

func upstreamPort(t *testing.T, upstream *fakeUpstream) int {
	t.Helper()
	_, portText, err := net.SplitHostPort(upstream.listener.Addr().String())
	if err != nil {
		t.Fatalf("split upstream addr: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse upstream port: %v", err)
	}
	return port
}

func startTestLane(t *testing.T, options Options) *Server {
	t.Helper()
	options.Host = "127.0.0.1"
	options.PreferredPort = 0
	options.PreferredKernelPort = 0
	ctx, cancel := context.WithCancel(context.Background())
	server, err := Start(ctx, options)
	if err != nil {
		cancel()
		t.Fatalf("start lane: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		shutdown, cancelTimeout := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelTimeout()
		_ = server.Close(shutdown)
	})
	return server
}

func getViaProxy(t *testing.T, proxyPort int, rawURL string) *http.Response {
	t.Helper()
	proxyURL, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(proxyPort))
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   5 * time.Second,
	}
	response, err := client.Get(rawURL)
	if err != nil {
		t.Fatalf("GET %s via proxy: %v", rawURL, err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}

func TestPlainRequestGoesThroughUpstream(t *testing.T) {
	upstream := startFakeUpstream(t)
	server := startTestLane(t, Options{
		Upstream: model.Upstream{Protocol: model.ProtocolHTTP, Host: "127.0.0.1", Port: upstreamPort(t, upstream)},
	})
	before := upstream.conns.Load()
	response := getViaProxy(t, server.InterfacePort(), "http://example.test/hello")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	body, _ := io.ReadAll(response.Body)
	if string(body) != "from-upstream" {
		t.Fatalf("body = %q", body)
	}
	if upstream.conns.Load() != before+1 {
		t.Fatalf("upstream connections = %d, want %d", upstream.conns.Load(), before+1)
	}
}

func TestDirectRouteBypassesUpstream(t *testing.T) {
	upstream := startFakeUpstream(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("direct-hit"))
	}))
	t.Cleanup(target.Close)
	_, portText, err := net.SplitHostPort(strings.TrimPrefix(target.URL, "http://"))
	if err != nil {
		t.Fatalf("split target addr: %v", err)
	}
	server := startTestLane(t, Options{
		Upstream: model.Upstream{Protocol: model.ProtocolHTTP, Host: "127.0.0.1", Port: upstreamPort(t, upstream)},
		Routes:   []model.RouteRule{{Pattern: "127.0.0.1", Action: model.RouteDirect}},
	})
	before := upstream.conns.Load()
	response := getViaProxy(t, server.InterfacePort(), "http://127.0.0.1:"+portText+"/")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	body, _ := io.ReadAll(response.Body)
	if string(body) != "direct-hit" {
		t.Fatalf("body = %q, want direct-hit", body)
	}
	if upstream.conns.Load() != before {
		t.Fatalf("upstream should not be contacted for direct route")
	}
}

func TestAllowlistRejectsUnknownHost(t *testing.T) {
	upstream := startFakeUpstream(t)
	var events []string
	var blocked model.RequestRecord
	server := startTestLane(t, Options{
		Upstream:       model.Upstream{Protocol: model.ProtocolHTTP, Host: "127.0.0.1", Port: upstreamPort(t, upstream)},
		HostFilterMode: model.HostFilterAllowlist,
		AllowedHosts:   []string{"*.openai.com"},
		OnRecord: func(event string, record model.RequestRecord) {
			events = append(events, event)
			if event == "request:new" {
				blocked = record
			}
		},
	})
	response := getViaProxy(t, server.InterfacePort(), "http://example.com/")
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.StatusCode)
	}
	if upstream.conns.Load() != 0 {
		t.Fatalf("blocked host must not reach upstream")
	}
	if len(events) < 2 || events[0] != "request:new" || events[1] != "request:finished" {
		t.Fatalf("blocked request events = %v, want new then finished", events)
	}
	if blocked.Result != "blocked" || blocked.Status != http.StatusForbidden || blocked.Host != "example.com" {
		t.Fatalf("blocked record = %+v", blocked)
	}
}
