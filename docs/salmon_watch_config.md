# Configuring Salmon-Watch

The default config file is `$XDG_CONFIG_HOME/salmon-watch/salmon-watch.yml`, or `~/.config/salmon-watch/salmon-watch.yml` when `XDG_CONFIG_HOME` isn't set.

To use another one:

```
$ salmon-watch --config /somewhere/salmon-watch.yml
```

YAML is parsed strictly. An unknown key causes an error instead of being silently ignored.

## Servers

The smallest useful config is:

```yaml
wsClient:
  servers:
    - id: local
      addr: localhost:41990
```

Every server has these fields:

### `id`

An arbitrary unique ID containing letters, digits, underscores, or hyphens. `internal` is reserved by Salmon-Watch.

The ID prefixes every incident received from that server, so it shouldn't be changed casually after the setup is already in use. Among other things, changing it also changes the keys used by saved snoozes.

### `addr`

The address Salmon-Watch connects to, in `host:port` form. With a tunnel, this is the local end of that tunnel, not the remote SSH host.

### Authentication

The simple config above is good for local monitoring, but for remote machines this isn't enough. Salmon supports SSH tunnel, TLS and bearer token, check [Security](./security.md) for details.
