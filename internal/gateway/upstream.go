package gateway

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
	xproxy "golang.org/x/net/proxy"
)

type UpstreamDialer struct {
	proxy   model.Upstream
	timeout time.Duration
}

func NewUpstreamDialer(upstream model.Upstream) *UpstreamDialer {
	return &UpstreamDialer{
		proxy:   upstream,
		timeout: 20 * time.Second,
	}
}

func (d *UpstreamDialer) Dial(ctx context.Context, target string) (net.Conn, error) {
	switch d.proxy.Protocol {
	case model.ProtocolHTTP:
		conn, err := d.dialTCP(ctx, d.proxyAddress())
		if err != nil {
			return nil, err
		}
		return d.connectThroughHTTP(conn, target)
	case model.ProtocolHTTPS:
		conn, err := d.dialTCP(ctx, d.proxyAddress())
		if err != nil {
			return nil, err
		}
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: d.proxy.Host,
			MinVersion: tls.VersionTLS12,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, fmt.Errorf("TLS handshake with upstream proxy: %w", err)
		}
		return d.connectThroughHTTP(tlsConn, target)
	case model.ProtocolSOCKS5:
		dialer, err := xproxy.SOCKS5("tcp", d.proxyAddress(), nil, &net.Dialer{Timeout: d.timeout})
		if err != nil {
			return nil, fmt.Errorf("create SOCKS5 dialer: %w", err)
		}
		type result struct {
			conn net.Conn
			err  error
		}
		ch := make(chan result, 1)
		go func() {
			conn, err := dialer.Dial("tcp", target)
			ch <- result{conn: conn, err: err}
		}()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case res := <-ch:
			if res.err != nil {
				return nil, fmt.Errorf("SOCKS5 connect %s: %w", target, res.err)
			}
			return res.conn, nil
		}
	default:
		return nil, fmt.Errorf("unsupported upstream protocol %q", d.proxy.Protocol)
	}
}

func (d *UpstreamDialer) dialTCP(ctx context.Context, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: d.timeout, KeepAlive: 30 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connect upstream %s: %w", address, err)
	}
	return conn, nil
}

func (d *UpstreamDialer) connectThroughHTTP(conn net.Conn, target string) (net.Conn, error) {
	deadline := time.Now().Add(d.timeout)
	_ = conn.SetDeadline(deadline)
	request := "CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\nProxy-Connection: keep-alive\r\n\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write CONNECT to upstream: %w", err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read CONNECT response: %w", err)
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	if response.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("upstream CONNECT returned %s", response.Status)
	}
	_ = conn.SetDeadline(time.Time{})
	if reader.Buffered() > 0 {
		return &bufferedConn{Conn: conn, reader: reader}, nil
	}
	return conn, nil
}

func (d *UpstreamDialer) proxyAddress() string {
	return net.JoinHostPort(d.proxy.Host, strconv.Itoa(d.proxy.Port))
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func ParseTarget(authority string, defaultPort int) (string, string, int, error) {
	host, portText, err := net.SplitHostPort(authority)
	if err != nil {
		if !strings.Contains(err.Error(), "missing port") {
			return "", "", 0, err
		}
		host = strings.Trim(authority, "[]")
		portText = strconv.Itoa(defaultPort)
	}
	if host == "" {
		return "", "", 0, errors.New("target host is empty")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", "", 0, fmt.Errorf("invalid target port %q", portText)
	}
	return host, net.JoinHostPort(host, strconv.Itoa(port)), port, nil
}
