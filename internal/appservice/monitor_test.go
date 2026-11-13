package appservice

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/platform"
)

func TestPendingFailoverRespectsUserChanges(t *testing.T) {
	for _, change := range []string{"unchanged", "manual selection", "disable failover", "stop lane"} {
		t.Run(change, func(t *testing.T) {
			service := newProxyTestService(t)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			accepted, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
					return
				}
				close(accepted)
				<-release
				_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
			}()
			service.cfg.AutoFailover = true
			backup := model.UpstreamProfile{Name: "backup", Protocol: model.ProtocolHTTP, Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port}
			manual := service.cfg.Upstreams[0]
			manual.Name = "manual"
			service.cfg.Upstreams = []model.UpstreamProfile{service.cfg.Upstreams[0], backup, manual}
			if err := service.StartLane(); err != nil {
				t.Fatal(err)
			}
			done := make(chan bool, 1)
			go func() { done <- service.failoverUpstream() }()
			select {
			case <-accepted:
			case <-time.After(time.Second):
				t.Fatal("failover probe did not start")
			}
			want := "primary"
			switch change {
			case "unchanged":
				want = "backup"
			case "manual selection":
				want = "manual"
				if _, err := service.SwitchUpstream(want); err != nil {
					t.Fatal(err)
				}
			case "disable failover":
				input := model.SettingsInput(config.Settings(service.currentConfig()))
				input.AutoFailover = false
				input.LaunchAtLogin = platform.IsAutoStart()
				if _, err := service.SaveSettings(input); err != nil {
					t.Fatal(err)
				}
			case "stop lane":
				service.StopLane()
			}
			release <- struct{}{}
			select {
			case switched := <-done:
				if switched != (change == "unchanged") || service.currentConfig().ActiveUpstream != want {
					t.Fatalf("stale failover overrode %s: %s", change, service.currentConfig().ActiveUpstream)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("failover did not finish")
			}
		})
	}
}
