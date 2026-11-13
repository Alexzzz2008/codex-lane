package gateway

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPProbeClosesItsIdleConnection(t *testing.T) {
	closed := make(chan struct{}, 1)
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "probe response")
	}))
	proxy.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	proxy.Start()
	defer proxy.Close()
	status, body, _, err := HTTPBodyThroughProxy(context.Background(), proxy.URL, "http://probe.example.test/")
	if err != nil || status != http.StatusOK || string(body) != "probe response" {
		t.Fatalf("probe response = %d %q, error = %v", status, body, err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("completed connectivity probe left its connection open")
	}
}
