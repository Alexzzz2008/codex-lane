package gateway

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestIPv6EndpointsUseBoundAddress(t *testing.T) {
	server, err := Start(context.Background(), Options{Host: "::1"})
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer server.Close(context.Background())
	client := &http.Client{Transport: &http.Transport{}}
	for _, address := range []string{server.InterfaceURL(), server.KernelURL()} {
		if !strings.HasPrefix(address, "http://[::1]:") {
			t.Fatalf("advertised incorrect address: %s", address)
		}
		response, err := client.Get(address + "/healthz")
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("health = %d", response.StatusCode)
		}
	}
}
