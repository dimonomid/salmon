package wsclient

import (
	"fmt"
	"regexp"
)

var validServerID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type Config struct {
	Servers []ConfigServer `yaml:"servers"`
}

type ConfigServer struct {
	// ID identifies this Salmon server and prefixes all incident keys received
	// from it. It must contain only letters, digits, underscores, or hyphens.
	ID string `yaml:"id"`

	// Addr is an address of the salmon server, in the form "host:port".
	Addr string `yaml:"addr"`
}

// Validate checks server IDs before they are used as map keys and incident-key
// prefixes.
func (c Config) Validate() error {
	serverIndexByID := make(map[string]int, len(c.Servers))
	for i, server := range c.Servers {
		if server.ID == "" {
			return fmt.Errorf("wsClient.servers[%d].id is required", i)
		}
		if !validServerID.MatchString(server.ID) {
			return fmt.Errorf("wsClient.servers[%d].id %q must contain only letters, digits, underscores, or hyphens", i, server.ID)
		}
		if server.ID == IDInternal {
			return fmt.Errorf("wsClient.servers[%d].id %q is reserved", i, server.ID)
		}
		if previousIndex, exists := serverIndexByID[server.ID]; exists {
			return fmt.Errorf("wsClient.servers[%d].id %q duplicates wsClient.servers[%d].id", i, server.ID, previousIndex)
		}
		serverIndexByID[server.ID] = i
	}
	return nil
}
