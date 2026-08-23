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
