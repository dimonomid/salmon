package wsclient

type Config struct {
	Servers []ConfigServer `yaml:"servers"`
}

type ConfigServer struct {
	// ID is an arbitrary id of this salmon server to be used by this client.
	// All incident keys from this server will be prefixed with that ID and a
	// dot.
	ID string `yaml:"id"`

	// Addr is an address of the salmon server, in the form "host:port".
	Addr string `yaml:"addr"`
}
