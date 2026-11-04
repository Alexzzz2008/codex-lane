package gateway

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func TestUpstreamHandshakeRespectsCancellation(t *testing.T) {
	for _, protocol := range []string{model.ProtocolHTTP, model.ProtocolSOCKS5} {
		t.Run(protocol, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			closed := make(chan struct{})
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				_, _ = io.Copy(io.Discard, conn)
				close(closed)
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			dialer := NewUpstreamDialer(model.Upstream{Protocol: protocol, Host: "127.0.0.1", Port: portOf(listener.Addr())})
			started := time.Now()
			conn, err := dialer.Dial(ctx, "example.test:443")
			if conn != nil {
				conn.Close()
			}
			if err == nil || time.Since(started) > time.Second {
				t.Fatalf("handshake did not respect timeout: %v (%s)", err, time.Since(started))
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("cancelled handshake left the upstream connection open")
			}
		})
	}
}

func TestConnectPreservesBufferedPayload(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, "buffered-payload-received")
	}))
	defer target.Close()
	server := startTestLane(t, Options{Routes: []model.RouteRule{{Pattern: "127.0.0.1", Action: model.RouteDirect}}})
	conn, err := net.Dial("tcp", server.interfaceL.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	authority := strings.TrimPrefix(target.URL, "http://")
	_, _ = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\nGET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", authority, authority, authority)
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status = %d", response.StatusCode)
	}
	response, err = http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("buffered payload was lost: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "buffered-payload-received" {
		t.Fatalf("payload response = %q, error = %v", body, err)
	}
}

func TestCloseClosesEstablishedConnectTunnel(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	closed := make(chan struct{})
	go func() {
		conn, err := target.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = io.Copy(io.Discard, conn)
		close(closed)
	}()
	server := startTestLane(t, Options{Routes: []model.RouteRule{{Pattern: "127.0.0.1", Action: model.RouteDirect}}})
	conn, err := net.Dial("tcp", server.interfaceL.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	_, _ = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target.Addr(), target.Addr())
	if _, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("stopping Lane left an established tunnel open")
	}
}

func TestConnectRejectsInvalidTarget(t *testing.T) {
	server := startTestLane(t, Options{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodConnect, "http://example.test", nil)
	request.Host = "example.test:99999"
	server.handler(model.SourceInterface).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid target status = %d, want 400", response.Code)
	}
}

func TestHTTPRecordsTimeToResponseHeaders(t *testing.T) {
	finished := make(chan model.RequestRecord, 1)
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(40 * time.Millisecond)
		_, _ = io.WriteString(writer, "response")
	}))
	defer target.Close()
	server := startTestLane(t, Options{
		Routes: []model.RouteRule{{Pattern: "127.0.0.1", Action: model.RouteDirect}},
		OnRecord: func(event string, record model.RequestRecord) {
			if event == "request:finished" {
				finished <- record
			}
		},
	})
	response := getViaProxy(t, server.InterfacePort(), target.URL)
	_, _ = io.Copy(io.Discard, response.Body)
	select {
	case record := <-finished:
		if record.LatencyMs < 30 || record.LatencyMs > record.DurationMs {
			t.Fatalf("invalid response latency: %+v", record)
		}
	case <-time.After(time.Second):
		t.Fatal("request completion was not recorded")
	}
}
