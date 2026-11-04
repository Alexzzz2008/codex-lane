package gateway

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func TestSwitchUpstreamPreservesExistingTunnelAndPorts(t *testing.T) {
	previous := startFakeUpstream(t)
	next := startFakeUpstream(t)
	server := startTestLane(t, Options{Upstream: model.Upstream{Protocol: model.ProtocolHTTP, Host: "127.0.0.1", Port: upstreamPort(t, previous)}})
	interfacePort, kernelPort, started := server.InterfacePort(), server.KernelPort(), server.StartedAt()
	conn, err := net.Dial("tcp", server.interfaceL.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = io.WriteString(conn, "CONNECT example.test:443 HTTP/1.1\r\nHost: example.test:443\r\n\r\n")
	reader := bufio.NewReader(conn)
	if _, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect}); err != nil {
		t.Fatal(err)
	}
	updated := model.Upstream{Protocol: model.ProtocolHTTP, Host: "127.0.0.1", Port: upstreamPort(t, next)}
	server.UpdateRouting(updated, nil, nil, model.HostFilterOff)
	_, _ = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: example.test\r\n\r\n")
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("switch interrupted the existing tunnel: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if previous.conns.Load() != 1 || next.conns.Load() != 0 {
		t.Fatal("established tunnel changed upstream")
	}
	for _, port := range []int{interfacePort, kernelPort} {
		response := getViaProxy(t, port, "http://example.test/new")
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
	}
	if next.conns.Load() != 2 {
		t.Fatalf("new requests did not use the new upstream: %d connections", next.conns.Load())
	}
	if server.InterfacePort() != interfacePort || server.KernelPort() != kernelPort || server.StartedAt() != started {
		t.Fatal("switch restarted the local proxy")
	}
}

func TestRoutingUpdateAppliesAllowlistWithoutRestart(t *testing.T) {
	upstream := startFakeUpstream(t)
	settings := model.Upstream{Protocol: model.ProtocolHTTP, Host: "127.0.0.1", Port: upstreamPort(t, upstream)}
	server := startTestLane(t, Options{Upstream: settings})
	allowed := []string{"allowed.test"}
	server.UpdateRouting(settings, nil, allowed, model.HostFilterAllowlist)
	allowed[0] = "blocked.test"
	response := getViaProxy(t, server.InterfacePort(), "http://blocked.test/")
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("new allowlist was not applied: %d", response.StatusCode)
	}
	response = getViaProxy(t, server.KernelPort(), "http://allowed.test/")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("allowlist changed through caller-owned slice: %d", response.StatusCode)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	server.UpdateRouting(settings, nil, nil, model.HostFilterOff)
	response = getViaProxy(t, server.InterfacePort(), "http://blocked.test/")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("disabling allowlist did not take effect: %d", response.StatusCode)
	}
}
