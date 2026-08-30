package wsclient_test

import (
	"strings"
	"testing"

	"github.com/dimonomid/salmon/wsclient"
)

func TestConfigValidateServerIDs(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		wantErr string
	}{
		{name: "none"},
		{name: "valid", ids: []string{"local", "remote-2", "backup_server"}},
		{name: "empty", ids: []string{""}, wantErr: `wsClient.servers[0].id is required`},
		{name: "whitespace", ids: []string{"remote server"}, wantErr: `wsClient.servers[0].id "remote server" must contain only`},
		{name: "dot", ids: []string{"remote.server"}, wantErr: `wsClient.servers[0].id "remote.server" must contain only`},
		{name: "reserved", ids: []string{"internal"}, wantErr: `wsClient.servers[0].id "internal" is reserved`},
		{name: "duplicate", ids: []string{"remote", "local", "remote"}, wantErr: `wsClient.servers[2].id "remote" duplicates wsClient.servers[0].id`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := wsclient.Config{Servers: make([]wsclient.ConfigServer, len(test.ids))}
			for i, id := range test.ids {
				config.Servers[i].ID = id
			}

			err := config.Validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() returned an error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want it to contain %q", err, test.wantErr)
			}
		})
	}
}

func TestNewCombinerRejectsInvalidServerID(t *testing.T) {
	_, err := wsclient.NewCombiner(wsclient.CombinerParams{
		Config: wsclient.Config{Servers: []wsclient.ConfigServer{{ID: "internal"}}},
	})
	if err == nil || !strings.Contains(err.Error(), `id "internal" is reserved`) {
		t.Fatalf("NewCombiner() error = %v, want reserved-ID error", err)
	}
}

func TestConfigValidateTunnels(t *testing.T) {
	validSSH := func() *wsclient.ConfigTunnel {
		return &wsclient.ConfigTunnel{SSH: &wsclient.ConfigSSHTunnel{
			Host: "example.com", User: "salmon", RemoteSalmonAddr: "127.0.0.1:41990",
		}}
	}
	tests := []struct {
		name    string
		server  wsclient.ConfigServer
		wantErr string
	}{
		{
			name:   "custom command",
			server: wsclient.ConfigServer{ID: "remote", Addr: "127.0.0.1:41992", Tunnel: &wsclient.ConfigTunnel{CustomCommand: &wsclient.ConfigCustomTunnelCommand{Command: []string{"ssh", "host"}}}},
		},
		{
			name:   "ssh",
			server: wsclient.ConfigServer{ID: "remote", Addr: "localhost:41992", Tunnel: validSSH()},
		},
		{
			name:    "empty tunnel",
			server:  wsclient.ConfigServer{ID: "remote", Addr: "localhost:41992", Tunnel: &wsclient.ConfigTunnel{}},
			wantErr: "must contain exactly one",
		},
		{
			name: "both tunnel forms",
			server: wsclient.ConfigServer{ID: "remote", Addr: "localhost:41992", Tunnel: &wsclient.ConfigTunnel{
				SSH: validSSH().SSH, CustomCommand: &wsclient.ConfigCustomTunnelCommand{Command: []string{"ssh"}},
			}},
			wantErr: "must contain exactly one",
		},
		{
			name:    "empty custom command",
			server:  wsclient.ConfigServer{ID: "remote", Addr: "localhost:41992", Tunnel: &wsclient.ConfigTunnel{CustomCommand: &wsclient.ConfigCustomTunnelCommand{Command: []string{}}}},
			wantErr: "must start with an executable",
		},
		{
			name: "empty readiness output",
			server: wsclient.ConfigServer{ID: "remote", Addr: "localhost:41992", Tunnel: &wsclient.ConfigTunnel{CustomCommand: &wsclient.ConfigCustomTunnelCommand{
				Command: []string{"ssh"}, ReadinessProbe: &wsclient.ConfigTunnelReadinessProbe{},
			}}},
			wantErr: "readinessProbe.containsOutput must not be empty",
		},
		{
			name: "missing SSH host",
			server: wsclient.ConfigServer{ID: "remote", Addr: "localhost:41992", Tunnel: &wsclient.ConfigTunnel{SSH: &wsclient.ConfigSSHTunnel{
				User: "salmon", RemoteSalmonAddr: "localhost:41990",
			}}},
			wantErr: ".ssh.host is required",
		},
		{
			name: "missing SSH user",
			server: wsclient.ConfigServer{ID: "remote", Addr: "localhost:41992", Tunnel: &wsclient.ConfigTunnel{SSH: &wsclient.ConfigSSHTunnel{
				Host: "example.com", RemoteSalmonAddr: "localhost:41990",
			}}},
			wantErr: ".ssh.user is required",
		},
		{
			name: "invalid SSH port",
			server: wsclient.ConfigServer{ID: "remote", Addr: "localhost:41992", Tunnel: &wsclient.ConfigTunnel{SSH: &wsclient.ConfigSSHTunnel{
				Host: "example.com", User: "salmon", Port: 65536, RemoteSalmonAddr: "localhost:41990",
			}}},
			wantErr: ".ssh.port",
		},
		{
			name:    "non-loopback local address",
			server:  wsclient.ConfigServer{ID: "remote", Addr: "0.0.0.0:41992", Tunnel: validSSH()},
			wantErr: "must use a loopback host",
		},
		{
			name: "invalid remote Salmon address",
			server: wsclient.ConfigServer{ID: "remote", Addr: "localhost:41992", Tunnel: &wsclient.ConfigTunnel{SSH: &wsclient.ConfigSSHTunnel{
				Host: "example.com", User: "salmon", RemoteSalmonAddr: "missing-port",
			}}},
			wantErr: "remoteSalmonAddr",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := (wsclient.Config{Servers: []wsclient.ConfigServer{test.server}}).Validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() returned an error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want it to contain %q", err, test.wantErr)
			}
		})
	}
}

func TestConfigValidateBearerAuth(t *testing.T) {
	tests := []struct {
		name    string
		server  wsclient.ConfigServer
		wantErr string
	}{
		{
			name:   "TLS",
			server: wsclient.ConfigServer{ID: "remote", Addr: "salmon.example.com:41990", TLS: &wsclient.ConfigTLS{}, Auth: &wsclient.ConfigAuth{BearerTokenFile: "/tmp/token"}},
		},
		{
			name:   "loopback plaintext",
			server: wsclient.ConfigServer{ID: "local", Addr: "127.0.0.1:41990", Auth: &wsclient.ConfigAuth{BearerTokenFile: "/tmp/token"}},
		},
		{
			name:   "remote plaintext",
			server: wsclient.ConfigServer{ID: "remote", Addr: "salmon.example.com:41990", Auth: &wsclient.ConfigAuth{BearerTokenFile: "/tmp/token"}},
		},
		{
			name:    "missing token file",
			server:  wsclient.ConfigServer{ID: "remote", Addr: "salmon.example.com:41990", TLS: &wsclient.ConfigTLS{}, Auth: &wsclient.ConfigAuth{}},
			wantErr: "auth.bearerTokenFile is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := (wsclient.Config{Servers: []wsclient.ConfigServer{test.server}}).Validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want it to contain %q", err, test.wantErr)
			}
		})
	}
}
