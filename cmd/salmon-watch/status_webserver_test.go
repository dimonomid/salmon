package main

import (
	"net"
	"testing"
)

func TestSetupWebserverListensOnlyOnIPv4Loopback(t *testing.T) {
	server := setupWebserver(newStatusWebserver(statusWebserverParams{}))
	t.Cleanup(func() { _ = server.Close() })

	address, ok := server.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address type %T", server.Addr())
	}
	if got, want := address.IP.String(), "127.0.0.1"; got != want {
		t.Fatalf("listener IP is %q, want %q", got, want)
	}
}
