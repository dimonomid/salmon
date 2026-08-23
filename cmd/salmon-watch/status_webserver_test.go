package main

import (
	"net"
	"testing"
)

func TestSetupWebserverListensOnlyOnIPv4Loopback(t *testing.T) {
	listener := setupWebserver(newStatusWebserver(statusWebserverParams{}))
	t.Cleanup(func() { _ = listener.Close() })

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address type %T", listener.Addr())
	}
	if got, want := address.IP.String(), "127.0.0.1"; got != want {
		t.Fatalf("listener IP is %q, want %q", got, want)
	}
}
