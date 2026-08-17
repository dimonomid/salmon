# Salmon

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
Syncthing incident, I finally resolved to implement an utility which would help
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
becomes "salmon". Yeah it's probably a weird name, but I didn't promise not to
be weird.

But wait a min, we're not yet done with the naming. As mentioned, the tool can
monitor not only local machine, but also server(s); so it means there should be
a server part (running on a remote machine and actually checking its health),
and also to get an icon on a desktop, there should be a client part.
"salmon-client" sounds boring. So if we think about it, what does one need to
make it easier to see a salmon? An
[aquascope](https://en.wikipedia.org/wiki/Aquascope). So yeah, you guessed it:
the client part is called "aquascope".

So all in all, we run `salmon` on every machine we want to monitor health for
(which can be local machine as well as any number of servers), and we run
`aquascope` on desktop, which will connect to all the `salmon`s it's configured
to check, and will show a tray icon.

As mentioned above, it's probably some weird naming. Forgive me.

## TODO install and setup
