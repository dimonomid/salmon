# Salmon: alerts and a tray icon for failing systemd services and anything else

Salmon is a relatively simple utility which monitors the health of your
server(s) and/or local machine, and helps you notice timely if something is
wrong.

## Project history and naming

I use [Syncthing](https://syncthing.net/) to sync important data across multiple
machines. It works so well so after I've set it up, after some time I obviously
just got used to it working all the time. And then one day I noticed that
apparently some stuff didn't get synced. So I checked around, and found out that
the Syncthing systemd service got broken a couple weeks ago for some reason,
without me noticing it, and so nothing was synced during this time. That was
really annoying: it means that the synced data on the machines that I use could
have get diverged if I changed it on both of them, and if it's binary data, then
merging the changes will likely be a challenge.

And what's even more annoying is that an incident like a systemd service failure
totally should have been communicated to me somehow, and yet I didn't know about
it until I noticed a side effect of this failure.

It wasn't the first time when I got frustrated about the OS being too silent
about failures like that (both locally and on the servers), and so after this
Syncthing incident, I finally resolved to implement a utility which would help
me notice systemd service failures, at least by showing a simple icon in tray,
just like a green/red dot.

So the first idea was to make it sort of "systemd monitoring". However, after a
short while I realized that I actually want to reuse the same tray icon for
monitoring things other than systemd services; a trivial example is to just
check if we're not running out of disk space. Most desktop environments do
perform this check for us, but when it comes to the servers, we're on our own;
and it happened to me multiple times in the past that a server runs out of disk
space and it takes a while to find that out and fix. So the next idea was, in
addition to monitoring systemd services health, to also support running some
arbitrary command periodically, and notify when the exit code is not what we
want. This way, we can implement "polling" of literally anything that can be
checked from the shell. This check is not as realtime as with systemd, since we
probably shouldn't poll things more frequently than once per minute, but for
things like checking disk space, it should be good enough.

Anyway, so then the name became something like "systemd et al monitoring".
Which, if I take some random letters out of it: "Systemd et AL MONitoring",
becomes "salmon".

Salmon has a server part (running locally and/or on remote machine(s) and
actually checking their health) and a desktop client part. The client is called
`salmon-watch`: it watches the status reported by one or more Salmon servers
via websocket, and provides a tray icon with a web UI.

So all in all, we run `salmon` on every machine we want to monitor health for
(which can be local machine as well as any number of servers), and we run
`salmon-watch` on desktop, which will connect to all the `salmon`s it's
configured to check and show a tray icon.

## Installation Quick Start

### Monitoring local machine

The easiest way to install both `salmon` and `salmon-watch` to monitor local
machine health is as follows:

```bash
# Download both prebuilt binaries from GitHub:
# TODO: fill it when prebuilt binaries are set up on github

# Move them wherever you normally keep locally managed executables (salmon
# needs to be a system binary, while salmon-watch can stay in home directory)
#
# For example (adjust as needed):
sudo install -m 755 salmon /usr/local/bin/salmon
install -m 755 salmon-watch ~/.local/bin/salmon-watch

# Let it create default configs, systemd service and desktop autostart entry.
sudo salmon setup
salmon-watch setup

# Start both:
sudo systemctl start salmon.service
salmon-watch
```

You should now see a tray icon, and if you click on it and then Status, you'll
see the web interface. When you reboot, it will start automatically.

### Monitoring remote machines

If you have e.g. a personal server which you also want to monitor using the
same interface on your desktop, then on each such remote machine follow the
same steps as above, but only for `salmon` (no need to install `salmon-watch`
on the servers).

By default, salmon only listens on `localhost`, so we can't simply connect to
its port remotely. See the longer discussion about authentication and security
below, but for now, since it's a "quick start" section, let's accept a
non-hardened setup, and configure it to listen on all interfaces: on the
server, open the file `/etc/salmon.yml`, and edit the
`messengers.webserver.listenAddress` key on the bottom: instead of
`127.0.0.1:41990`, it should become just `:41990`

After that, restart the service:

```bash
sudo systemctl restart salmon.service
```

Having that done, we can point the desktop `salmon-watch` at our remote
machine: open the config file `~/.config/salmon-watch/salmon-watch.yml`, and
add one more entry to the `wsClient.servers` array, like that:

```yaml
    - id: myserver # Arbitrary but unique ID for this server.
      addr: something.com:41990 # Replace something.com with the actual IP or hostname.
```

And restart `salmon-watch`. Open its web UI and verify that the list of servers
now includes your newly added remote server as well.

## Default config

The default config (which `sudo salmon setup` writes to `/etc/salmon.yml`) is
as follows: if any systemd service is failing, it's a warning (the tray icon
will be blinking yellow). If there's less than 100 MiB of free space in the
root partition, it's an error (the tray icon will be blinking red). Otherwise,
it's all good (the tray icon is green).

The config has comments and examples, so check it out and play with it. For
example, I like to list some services that I particularly care about (such as
syncthing) for which I want to trigger an error, not a warning. And I have some
more custom exec checks as well.

Don't forget to restart the systemd service to apply the changes.

## Authentication and security

In the quick start section above, we configured the remote server to listen on
all interfaces without any authentication, so it means technically anyone could
find that out and connect to it and watch incidents on your machine.

The risk is not huge: the salmon API is read-only, and the info about some
failed services is likely not particularly sensitive, but still it's obviously
much more exposed than it should be, and so it's not a good idea to leave it
running like that forever.

TODO: explain that the authentication would only be useful if the transport is
encrypted, which requires SSL or WireGuard, and if we go this far, it's easier to
just use an SSH tunnel.
