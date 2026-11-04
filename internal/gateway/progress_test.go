package gateway

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestTunnelReportsTrafficBeforeConnectionCloses(t *testing.T) {
	client, clientRelay := net.Pipe()
	upstreamRelay, target := net.Pipe()
	defer client.Close()
	defer target.Close()
	progress := make(chan [2]int64, 4)
	finished := make(chan [2]int64, 1)
	go func() {
		up, down := tunnel(clientRelay, upstreamRelay, func(up, down int64) { progress <- [2]int64{up, down} })
		finished <- [2]int64{up, down}
	}()
	go func() { buffer := make([]byte, 4); _, _ = io.ReadFull(target, buffer); _, _ = target.Write(buffer) }()
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(client, make([]byte, 4)); err != nil {
		t.Fatal(err)
	}
	select {
	case counts := <-progress:
		if counts != [2]int64{4, 4} {
			t.Fatalf("incorrect live counters: %v", counts)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("traffic was withheld until connection closed")
	}
	client.Close()
	select {
	case counts := <-finished:
		if counts != [2]int64{4, 4} {
			t.Fatalf("incorrect final counters: %v", counts)
		}
	case <-time.After(time.Second):
		t.Fatal("tunnel did not stop")
	}
}
