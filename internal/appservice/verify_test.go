package appservice

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDirectProbeClosesItsIdleConnection(t *testing.T) {
	closed := make(chan struct{}, 1)
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	target.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	target.Start()
	defer target.Close()
	status, _, err := directHTTPStatus(context.Background(), target.URL)
	if err != nil || status != http.StatusOK {
		t.Fatalf("direct probe = %d, error = %v", status, err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("completed direct probe left its connection open")
	}
}
