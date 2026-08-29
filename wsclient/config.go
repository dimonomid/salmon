package wsclient

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
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

	// TLS enables a secure WebSocket connection when present.
	TLS *ConfigTLS `yaml:"tls,omitempty"`

	// Tunnel optionally runs a persistent local tunnel command for this server.
	Tunnel *ConfigTunnel `yaml:"tunnel,omitempty"`
}

// ConfigTLS controls certificate verification for a secure WebSocket
// connection. An empty value uses the operating system's trusted CAs and the
// hostname from ConfigServer.Addr.
type ConfigTLS struct {
	// CAFile is an optional path to additional PEM-encoded CA certificates to trust.
	CAFile string `yaml:"caFile,omitempty"`

	// ServerName optionally overrides the hostname verified in the server
	// certificate. This is useful when Addr points at a local tunnel.
	ServerName string `yaml:"serverName,omitempty"`
}

// ConfigTunnel describes a tunnel to the server. Exactly one field must be non-nil.
type ConfigTunnel struct {
	// SSH builds a standard local-forwarding command.
	SSH *ConfigSSHTunnel `yaml:"ssh,omitempty"`

	// CustomCommand describes any persistent command that provides the local
	// tunnel.
	CustomCommand *ConfigCustomTunnelCommand `yaml:"customCommand,omitempty"`
}

type ConfigCustomTunnelCommand struct {
	// Command contains an executable followed by its arguments. It is run
	// directly without a shell and must remain running while the tunnel is up.
	Command []string `yaml:"command"`

	// ReadinessProbe delays WebSocket connections until the command reports
	// readiness. When omitted, the tunnel is considered ready immediately after
	// the command starts.
	ReadinessProbe *ConfigTunnelReadinessProbe `yaml:"readinessProbe,omitempty"`
}

type ConfigTunnelReadinessProbe struct {
	// ContainsOutput marks the command ready when either stdout or stderr
	// contains this text.
	ContainsOutput string `yaml:"containsOutput"`
}

type ConfigSSHTunnel struct {
	Host string `yaml:"host"`
	User string `yaml:"user"`
	// Port is the SSH server port. It defaults to 22.
	Port int `yaml:"port"`
	// RemoteSalmonAddr is the address the SSH server uses to reach Salmon.
	RemoteSalmonAddr string `yaml:"remoteSalmonAddr"`
	// ExtraSSHArgs are inserted as individual ssh arguments without shell
	// interpretation.
	ExtraSSHArgs []string `yaml:"extraSshArgs,omitempty"`
}

// Validate checks server IDs and optional tunnel configurations.
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
		if err := validateTunnel(server, i); err != nil {
			return err
		}
	}
	return nil
}

func validateTunnel(server ConfigServer, serverIndex int) error {
	if server.Tunnel == nil {
		return nil
	}
	prefix := fmt.Sprintf("wsClient.servers[%d].tunnel", serverIndex)
	hasSSH := server.Tunnel.SSH != nil
	hasCustomCommand := server.Tunnel.CustomCommand != nil
	if hasSSH == hasCustomCommand {
		return fmt.Errorf("%s must contain exactly one of ssh or customCommand", prefix)
	}
	if hasCustomCommand {
		custom := server.Tunnel.CustomCommand
		if len(custom.Command) == 0 || custom.Command[0] == "" {
			return fmt.Errorf("%s.customCommand.command must start with an executable", prefix)
		}
		if custom.ReadinessProbe != nil && custom.ReadinessProbe.ContainsOutput == "" {
			return fmt.Errorf("%s.customCommand.readinessProbe.containsOutput must not be empty", prefix)
		}
		return nil
	}

	ssh := server.Tunnel.SSH
	if ssh.Host == "" {
		return fmt.Errorf("%s.ssh.host is required", prefix)
	}
	if strings.HasPrefix(ssh.Host, "-") {
		return fmt.Errorf("%s.ssh.host must not start with a hyphen", prefix)
	}
	if ssh.User == "" {
		return fmt.Errorf("%s.ssh.user is required", prefix)
	}
	if strings.HasPrefix(ssh.User, "-") {
		return fmt.Errorf("%s.ssh.user must not start with a hyphen", prefix)
	}
	if ssh.Port < 0 || ssh.Port > 65535 {
		return fmt.Errorf("%s.ssh.port must be between 1 and 65535 when set", prefix)
	}
	if ssh.RemoteSalmonAddr == "" {
		return fmt.Errorf("%s.ssh.remoteSalmonAddr is required", prefix)
	}
	if err := validateHostPort(ssh.RemoteSalmonAddr); err != nil {
		return fmt.Errorf("%s.ssh.remoteSalmonAddr: %w", prefix, err)
	}
	if err := validateHostPort(server.Addr); err != nil {
		return fmt.Errorf("wsClient.servers[%d].addr for an SSH tunnel: %w", serverIndex, err)
	}
	localHost, _, _ := net.SplitHostPort(server.Addr)
	if localHost != "localhost" {
		ip := net.ParseIP(localHost)
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("wsClient.servers[%d].addr must use a loopback host for an SSH tunnel", serverIndex)
		}
	}
	return nil
}

func validateHostPort(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("must be a valid host:port: %w", err)
	}
	if host == "" {
		return fmt.Errorf("host must not be empty")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}
