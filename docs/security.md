# Security

Overall there are three acceptable ways to set up `salmon-watch` and `salmon` communication:

1. `salmon-watch` -> loopback connection -> locally-running `salmon` listening on `127.0.0.1` - this is the default and most straightforward setup. No authentication or encryption is needed here, since the communication never leaves the local machine.
2. `salmon-watch` -> ssh tunnel to a server -> `salmon` running on a server listening on `127.0.0.1` - this is the easiest to setup for a remote server, provided that you have an ssh access and you use public key authentication (i.e. you don't have to enter password). In this case, all the authentication and encryption is delegated to `ssh`; salmon itself still doesn't require any authentication.
3. `salmon-watch` -> TLS connection to a server -> `salmon` running on a server listening on public interface, and requiring bearer token authentication - this is an alternative way to setup secure communication: TLS provides server authentication and encryption, while bearer token provides client authentication.

Now let's talk about these in more detail.

## Loopback connection

Not much to say here, it's the default configuration.

The `salmon` webserver configuration looks like this - just listening on a local port without requiring any authentication:

```yaml
  messengers:
    - webserver:
        listenAddress: "127.0.0.1:41990"
```

And `salmon-watch` configuration is equally boring:

```yaml
wsClient:
  servers:
    - id: local # Arbitrary but unique ID for this server.
      addr: localhost:41990
```

## Remote salmon via ssh tunnel

This is the easiest to setup for a remote server, provided that you have an ssh access and you use public key authentication (i.e. you don't have to enter password). In this case, all the authentication and encryption is delegated to `ssh`; salmon itself still doesn't require any authentication.

The `salmon` config stays exactly as it is with the loopback connection - simply listening on a local port without requiring any authentication, because from the `salmon`'s point of view, the connection is still local.

While `salmon-watch` configuration should specify the tunnel:

```yaml
    - id: myserver            # Arbitrary but unique ID for this server.
      addr: localhost:42990   # Just any available port on the local machine
      tunnel:
        ssh:
          host: myserver.com  # TODO: Your actual server hostname
          user: myuser        # TODO: Your actual ssh user
          port: 22            # Change if using non-default ssh port
          remoteSalmonAddr: 127.0.0.1:41990
```

Having this, `salmon-watch` will spawn an external `ssh` process forwarding the remote port 41990 to the local port 42990, and once the tunnel is ready, connect to that local port. The ssh command will be something like this:

```
ssh -N -T \
  -o BatchMode=yes \
  -o ExitOnForwardFailure=yes -o ConnectTimeout=15 \
  -o ServerAliveInterval=10 -o ServerAliveCountMax=3 \
  -o PermitLocalCommand=yes -o "LocalCommand=echo SALMON_TUNNEL_READY" \
  -p 22 \
  -L localhost:42990:127.0.0.1:41990 \
  myuser@myserver.com
```

If you need to pass some extra arguments to `ssh`, such as to specify a
specific private key to use or anything else, you can specify them using the
`extraSshArgs` field, like that:

```yaml
      tunnel:
        ssh:
          # .... all the field as shown above
          extraSshArgs:
            - "-i"
            - "/path/to/specific/private.key"
```

And they will be added to the ssh command.

You can also establish the tunnel manually if you want, and get the same result, but I find it convenient to let `salmon-watch` manage the tunnel for me.

## Remote salmon via custom tunnel command

You could use any custom command to establish a tunnel, like that:

```yaml
    - id: myserver            # Arbitrary but unique ID for this server.
      addr: localhost:42990   # Just any available port on the local machine
      tunnel:
        customCommand:
          command: [
            # Any arbitrary command can go here, like your custom script or whatever.
            # For this example, we again use ssh.
            "ssh",
            "-N", "-T",
            "-o", "BatchMode=yes",
            "-o", "PermitLocalCommand=yes",
            "-o", "LocalCommand=echo MY_TUNNEL_IS_READY",
            # ... whatever other arguments you need
            "-L", "localhost:42990:127.0.0.1:41990",
            "myuser@myserver.com"
          ]
          readinessProbe:
            containsOutput: "MY_TUNNEL_IS_READY"
```

As you see, we're specifying a raw command to be executed, and optionally also a substring to watch for in the output which would mean that the tunnel is ready to use. If the `readinessProbe` isn't provided, the tunnel is considered ready right after command start.

Make sure that the custom tunnel command does not spawn subprocesses, and that when the tunnel is dead, the command should exit.

## Remote salmon via TLS and bearer token

This is a bit more involved to set up, so before you go there, make sure you're familiar with the simpler alternatives explained above.

In this setup, `salmon` listens on a public interface, so both TLS and authentication should be configured: TLS encrypts the connection and makes sure that `salmon-watch` is talking to the expected server, while the bearer token lets `salmon` authenticate `salmon-watch`.

First, let's setup the TLS part.

### Setting up TLS

You need a TLS certificate and its private key on the server. The user which runs `salmon` service (in the default setup, the user is also named `salmon`) must be able to read both files.

If you don't have an existing certificate that you can use, you can create a self-signed one, like that (optionally replace `myserverforcert.com` with whatever hostname you want to use in the certificate, and also adjust the expiration `-days` as you need):

```bash
sudo mkdir -p /etc/salmon/tls
sudo chown root:salmon /etc/salmon/tls
sudo chmod 0750 /etc/salmon/tls

sudo openssl req -x509 -newkey rsa:3072 -sha256 -days 3650 -nodes \
  -keyout /etc/salmon/tls/privkey.pem \
  -out /etc/salmon/tls/cert.pem \
  -subj "/CN=myserverforcert.com" \
  -addext "subjectAltName=DNS:myserverforcert.com"

sudo chown root:salmon /etc/salmon/tls/privkey.pem /etc/salmon/tls/cert.pem
sudo chmod 0640 /etc/salmon/tls/privkey.pem /etc/salmon/tls/cert.pem
```

In the end, with a normal certificate or a self-signed one, the `salmon` webserver configuration could look like this:

```yaml
  messengers:
    - webserver:
        listenAddress: "0.0.0.0:41990"
        tls:
          certFile: "/path/to/cert.pem"     # TODO: use actual path
          keyFile: "/path/to/privkey.pem"   # TODO: use actual path
```

On the `salmon-watch` side, we need to specify that we want to use TLS. If the server certificate is issued by a CA trusted by your operating system, we just need to add an empty `tls` object to the corresponding server:

```yaml
wsClient:
  servers:
    - id: myserver
      addr: myserver.com:41990
      tls: {}
```

If the certificate was self-signed though, or if hostname in `addr` is different from the hostname in the certificate, you need to specify details, like that:

```yaml
      tls:
        caFile: "/path/to/cert.pem"   # A copy of the self-signed cert.pem
        serverName: myserverforcert.com      # Hostname used in the certificate
```

With that, TLS should be set up now, and we move on to the bearer token.

### Setting up bearer token

Salmon-Watch has a convenient command for this:

```
salmon-watch generate-bearer-token myserver
```

Here `myserver` is the ID of the corresponding server in the `salmon-watch` configuration. The command creates a token file with owner-only permissions and prints the exact configuration snippets to add on both sides. `salmon-watch` stores the token itself, while `salmon` only stores its SHA-256 hash.

As the command output tells us, we need to add the `auth` object to `salmon-watch` config:

```yaml
wsClient:
  servers:
    - id: myserver
      addr: myserver.com:41990
      tls: {} # Or whatever you had there
      auth:
        bearerTokenFile: "/path/to/myserver.token"
```

And on the server side, also add `auth` with the corresponding token hash, so it ends up looking like this:

```yaml
  messengers:
    - webserver:
        listenAddress: "0.0.0.0:41990"
        tls:
          certFile: "/path/to/cert.pem"
          keyFile: "/path/to/privkey.pem"

        auth:
          - id: my-laptop # Identifies this credential; adjust as needed.
            bearerTokenHash: "sha256:..."
```
