package webserver

type Config struct {
	ListenAddress string `yaml:"listenAddress"`

	// TLS enables HTTPS and secure WebSockets when present.
	TLS *ConfigTLS `yaml:"tls,omitempty"`
}

// ConfigTLS identifies the certificate and private key presented by the
// Salmon webserver.
type ConfigTLS struct {
	// CertFile is the path to the PEM-encoded server certificate chain.
	CertFile string `yaml:"certFile"`

	// KeyFile is the path to the PEM-encoded private key for CertFile.
	KeyFile string `yaml:"keyFile"`
}
