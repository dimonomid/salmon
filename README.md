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

TODO: fix concurrent map write

/home/dmitrii
Connecting to ws://161.35.34.79:60021/api/v1/wsconnect
Connecting to ws://localhost:48173/api/v1/wsconnect
Connecting to ws://server.dmitryfrank.com:60019/api/v1/wsconnect
fatal error: concurrent map writes

goroutine 7 [running]:
runtime.throw({0x6b7d2e, 0x50})
	/usr/local/go/src/runtime/panic.go:1198 +0x71 fp=0xc00009c890 sp=0xc00009c860 pc=0x4361d1
runtime.mapassign_faststr(0x671c20, 0xc000076510, {0xc0000a4030, 0x23})
	/usr/local/go/src/runtime/map_faststr.go:294 +0x38b fp=0xc00009c8f8 sp=0xc00009c890 pc=0x415beb
github.com/dimonomid/salmon/statestracker.(*ItemStatesTracker).FeedItems(0xc000060140, 0xc00009ce50)
	/home/dmitrii/mydata/projects/salmon/statestracker/item_states_tracker.go:44 +0x2dd fp=0xc00009cda0 sp=0xc00009c8f8 pc=0x5da73d
github.com/dimonomid/salmon/wsclient.(*Combiner).runWSClient(0xc0000780a0, {{0xc0000181d0, 0x0}, {0xc00001c340, 0x0}}, 0xc00006c480, 0xc00006c4e0, 0xc00006c540)
	/home/dmitrii/mydata/projects/salmon/wsclient/combiner.go:133 +0x365 fp=0xc00009cf90 sp=0xc00009cda0 pc=0x6192c5
github.com/dimonomid/salmon/wsclient.NewCombiner·dwrap·1()
	/home/dmitrii/mydata/projects/salmon/wsclient/combiner.go:54 +0x45 fp=0xc00009cfe0 sp=0xc00009cf90 pc=0x618885
runtime.goexit()
	/usr/local/go/src/runtime/asm_amd64.s:1581 +0x1 fp=0xc00009cfe8 sp=0xc00009cfe0 pc=0x465301
created by github.com/dimonomid/salmon/wsclient.NewCombiner
	/home/dmitrii/mydata/projects/salmon/wsclient/combiner.go:54 +0x350

goroutine 1 [syscall, locked to thread]:
github.com/dimonomid/systray._Cfunc_nativeLoop()
	_cgo_gotypes.go:117 +0x48
github.com/dimonomid/systray.nativeLoop(...)
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/dimonomid/systray@v0.0.0-20190215110736-5927fc61b2c3/systray_nonwindows.go:19
github.com/dimonomid/systray.Run(0x6cc3a0, 0x6cc390)
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/dimonomid/systray@v0.0.0-20190215110736-5927fc61b2c3/systray.go:77 +0x16b
main.main()
	/home/dmitrii/mydata/projects/salmon/cmd/aquascope/main.go:43 +0x27

goroutine 21 [runnable]:
context.(*cancelCtx).Done(0xc0001524c0)
	/usr/local/go/src/context/context.go:358 +0x1e5
net.cgoLookupIP({0x714ff8, 0xc0001524c0}, {0x6b2568, 0x9}, {0xc000126215, 0xc000046800})
	/usr/local/go/src/net/cgo_unix.go:234 +0x16b
net.(*Resolver).lookupIP(0x8c2a20, {0x714ff8, 0xc0001524c0}, {0x6b2568, 0x3}, {0xc000126215, 0x9})
	/usr/local/go/src/net/lookup_unix.go:97 +0x128
net.glob..func1({0x714ff8, 0xc0001524c0}, 0x0, {0x6b2568, 0x0}, {0xc000126215, 0x0})
	/usr/local/go/src/net/hook.go:23 +0x3d
net.(*Resolver).lookupIPAddr.func1()
	/usr/local/go/src/net/lookup.go:296 +0x9f
internal/singleflight.(*Group).doCall(0x8c2a30, 0xc0001003c0, {0xc00012c940, 0xd}, 0xc000028340)
	/usr/local/go/src/internal/singleflight/singleflight.go:95 +0x3b
created by internal/singleflight.(*Group).DoChan
	/usr/local/go/src/internal/singleflight/singleflight.go:88 +0x2f1

goroutine 5 [select]:
github.com/dimonomid/salmon/wsclient.(*Combiner).runWSClient(0xc0000780a0, {{0xc000018198, 0x1}, {0xc0000181b0, 0x63c772}}, 0xc00006c360, 0xc00006c3c0, 0xc00006c420)
	/home/dmitrii/mydata/projects/salmon/wsclient/combiner.go:120 +0x125
created by github.com/dimonomid/salmon/wsclient.NewCombiner
	/home/dmitrii/mydata/projects/salmon/wsclient/combiner.go:54 +0x350

goroutine 6 [select]:
net.(*Resolver).lookupIPAddr(0x8c2a20, {0x715068, 0xc00012a480}, {0x6b2568, 0x0}, {0xc000126215, 0x9})
	/usr/local/go/src/net/lookup.go:302 +0x5c7
net.(*Resolver).internetAddrList(0x715068, {0x715068, 0xc00012a480}, {0x6b2568, 0x3}, {0xc000126215, 0xf})
	/usr/local/go/src/net/ipsock.go:288 +0x67a
net.(*Resolver).resolveAddrList(0x8f4550, {0x715068, 0xc00012a480}, {0x6b2745, 0x4}, {0x6b2568, 0x43e8fb}, {0xc000126215, 0xf}, {0x0, ...})
	/usr/local/go/src/net/dial.go:221 +0x41b
net.(*Dialer).DialContext(0xc00012a4e0, {0x715068, 0xc00012a480}, {0x6b2568, 0xc00012c000}, {0xc000126215, 0x7f17eaea0ca8})
	/usr/local/go/src/net/dial.go:406 +0x448
github.com/gorilla/websocket.(*Dialer).DialContext.func2({0x6b2568, 0xc000051a48}, {0xc000126215, 0xc000126215})
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/gorilla/websocket@v1.4.2/client.go:249 +0x45
github.com/gorilla/websocket.(*Dialer).DialContext.func3({0x6b2568, 0x6740e0}, {0xc000126215, 0x15})
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/gorilla/websocket@v1.4.2/client.go:257 +0x47
github.com/gorilla/websocket.(*Dialer).DialContext(0x2, {0x715030, 0xc00012c000}, {0xc000126210, 0x25}, 0x0)
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/gorilla/websocket@v1.4.2/client.go:291 +0x12ae
github.com/gorilla/websocket.(*Dialer).Dial(0x70f8e0, {0xc000126210, 0xc000044708}, 0x2)
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/gorilla/websocket@v1.4.2/client.go:106 +0x38
github.com/dimonomid/salmon/wsclient.(*WSClient).eventLoop(0xc000028340)
	/home/dmitrii/mydata/projects/salmon/wsclient/wsclient.go:95 +0x1c5
created by github.com/dimonomid/salmon/wsclient.New
	/home/dmitrii/mydata/projects/salmon/wsclient/wsclient.go:66 +0xff

goroutine 8 [select]:
net.(*Resolver).lookupIPAddr(0x8c2a20, {0x715068, 0xc00008e180}, {0x6b2568, 0xc00009d6b8}, {0xc0000a6005, 0x16})
	/usr/local/go/src/net/lookup.go:302 +0x5c7
net.(*Resolver).internetAddrList(0x715068, {0x715068, 0xc00008e180}, {0x6b2568, 0x3}, {0xc0000a6005, 0x1c})
	/usr/local/go/src/net/ipsock.go:288 +0x67a
net.(*Resolver).resolveAddrList(0x8f4550, {0x715068, 0xc00008e180}, {0x6b2745, 0x4}, {0x6b2568, 0x1}, {0xc0000a6005, 0x1c}, {0x0, ...})
	/usr/local/go/src/net/dial.go:221 +0x41b
net.(*Dialer).DialContext(0xc00008e1e0, {0x715068, 0xc00008e180}, {0x6b2568, 0xc00012c000}, {0xc0000a6005, 0x7f17eae9e5e0})
	/usr/local/go/src/net/dial.go:406 +0x448
github.com/gorilla/websocket.(*Dialer).DialContext.func2({0x6b2568, 0xc00009da48}, {0xc0000a6005, 0xc0000a6005})
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/gorilla/websocket@v1.4.2/client.go:249 +0x45
github.com/gorilla/websocket.(*Dialer).DialContext.func3({0x6b2568, 0x6740e0}, {0xc0000a6005, 0x15})
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/gorilla/websocket@v1.4.2/client.go:257 +0x47
github.com/gorilla/websocket.(*Dialer).DialContext(0x2, {0x715030, 0xc00012c000}, {0xc0000a6000, 0x32}, 0x0)
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/gorilla/websocket@v1.4.2/client.go:291 +0x12ae
github.com/gorilla/websocket.(*Dialer).Dial(0x70f8e0, {0xc0000a6000, 0xc00009df08}, 0x2)
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/gorilla/websocket@v1.4.2/client.go:106 +0x38
github.com/dimonomid/salmon/wsclient.(*WSClient).eventLoop(0xc000028380)
	/home/dmitrii/mydata/projects/salmon/wsclient/wsclient.go:95 +0x1c5
created by github.com/dimonomid/salmon/wsclient.New
	/home/dmitrii/mydata/projects/salmon/wsclient/wsclient.go:66 +0xff

goroutine 9 [select]:
github.com/dimonomid/salmon/wsclient.(*Combiner).runWSClient(0xc0000780a0, {{0xc000018200, 0x0}, {0xc00001a300, 0x0}}, 0xc00006c5a0, 0xc00006c600, 0xc00006c660)
	/home/dmitrii/mydata/projects/salmon/wsclient/combiner.go:120 +0x125
created by github.com/dimonomid/salmon/wsclient.NewCombiner
	/home/dmitrii/mydata/projects/salmon/wsclient/combiner.go:54 +0x350

goroutine 10 [IO wait]:
internal/poll.runtime_pollWait(0x7f17e81f8e88, 0x77)
	/usr/local/go/src/runtime/netpoll.go:229 +0x89
internal/poll.(*pollDesc).wait(0xc000210200, 0xc000076660, 0x0)
	/usr/local/go/src/internal/poll/fd_poll_runtime.go:84 +0x32
internal/poll.(*pollDesc).waitWrite(...)
	/usr/local/go/src/internal/poll/fd_poll_runtime.go:93
internal/poll.(*FD).WaitWrite(...)
	/usr/local/go/src/internal/poll/fd_unix.go:529
net.(*netFD).connect(0xc000210200, {0x715068, 0xc00006c6c0}, {0xc000133420, 0x4100d4}, {0x70fa20, 0xc00001c480})
	/usr/local/go/src/net/fd_unix.go:142 +0x717
net.(*netFD).dial(0xc000210200, {0x715068, 0xc00006c6c0}, {0x716b28, 0x0}, {0x716b28, 0xc000076600}, 0xc000133610)
	/usr/local/go/src/net/sock_posix.go:150 +0x379
net.socket({0x715068, 0xc00006c6c0}, {0x6b2568, 0x3}, 0x2, 0x1, 0x0, 0x10, {0x716b28, 0x0}, ...)
	/usr/local/go/src/net/sock_posix.go:71 +0x2a5
net.internetSocket({0x715068, 0xc00006c6c0}, {0x6b2568, 0x3}, {0x716b28, 0x0}, {0x716b28, 0xc000076600}, 0xc000018290, 0x0, ...)
	/usr/local/go/src/net/ipsock_posix.go:142 +0xf8
net.(*sysDialer).doDialTCP(0xc000210180, {0x715068, 0xc00006c6c0}, 0x0, 0x673fc0)
	/usr/local/go/src/net/tcpsock_posix.go:66 +0xa5
net.(*sysDialer).dialTCP(0xc00006c6c0, {0x715068, 0xc00006c6c0}, 0x4cb446, 0x0)
	/usr/local/go/src/net/tcpsock_posix.go:62 +0x59
net.(*sysDialer).dialSingle(0xc000210180, {0x715068, 0xc00006c6c0}, {0x7134e0, 0xc000076600})
	/usr/local/go/src/net/dial.go:583 +0x28b
net.(*sysDialer).dialSerial(0xc000210180, {0x715068, 0xc00006c6c0}, {0xc000012360, 0x1, 0x6b2745})
	/usr/local/go/src/net/dial.go:551 +0x312
net.(*Dialer).DialContext(0xc00006c720, {0x715068, 0xc00006c6c0}, {0x6b2568, 0xc00012c000}, {0xc000016275, 0x476e80})
	/usr/local/go/src/net/dial.go:428 +0x736
github.com/gorilla/websocket.(*Dialer).DialContext.func2({0x6b2568, 0xc000055a48}, {0xc000016275, 0xc000016275})
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/gorilla/websocket@v1.4.2/client.go:249 +0x45
github.com/gorilla/websocket.(*Dialer).DialContext.func3({0x6b2568, 0x6740e0}, {0xc000016275, 0x15})
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/gorilla/websocket@v1.4.2/client.go:257 +0x47
github.com/gorilla/websocket.(*Dialer).DialContext(0x2, {0x715030, 0xc00012c000}, {0xc000016270, 0x28}, 0x0)
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/gorilla/websocket@v1.4.2/client.go:291 +0x12ae
github.com/gorilla/websocket.(*Dialer).Dial(0x70f8e0, {0xc000016270, 0xc000226708}, 0x2)
	/home/dmitrii/mydata/projects/go/pkg/mod/github.com/gorilla/websocket@v1.4.2/client.go:106 +0x38
github.com/dimonomid/salmon/wsclient.(*WSClient).eventLoop(0xc0000283c0)
	/home/dmitrii/mydata/projects/salmon/wsclient/wsclient.go:95 +0x1c5
created by github.com/dimonomid/salmon/wsclient.New
	/home/dmitrii/mydata/projects/salmon/wsclient/wsclient.go:66 +0xff

goroutine 20 [chan receive]:
main.onReady.func2()
	/home/dmitrii/mydata/projects/salmon/cmd/aquascope/main.go:139 +0x32
created by main.onReady
	/home/dmitrii/mydata/projects/salmon/cmd/aquascope/main.go:133 +0x588

goroutine 34 [runnable]:
sync.runtime_SemacquireMutex(0x41a54b, 0x0, 0x0)
	/usr/local/go/src/runtime/sema.go:71 +0x25
sync.(*Mutex).lockSlow(0x8f45f4)
	/usr/local/go/src/sync/mutex.go:138 +0x165
sync.(*Mutex).Lock(...)
	/usr/local/go/src/sync/mutex.go:81
sync.(*Once).doSlow(0x8, 0x6cc428)
	/usr/local/go/src/sync/once.go:64 +0x53
sync.(*Once).Do(...)
	/usr/local/go/src/sync/once.go:59
net.systemConf(...)
	/usr/local/go/src/net/conf.go:43
net.(*Resolver).lookupIP(0x8c2a20, {0x714ff8, 0xc0000c6000}, {0x6b2568, 0x3}, {0xc0000a6005, 0x16})
	/usr/local/go/src/net/lookup_unix.go:95 +0xbe
net.glob..func1({0x714ff8, 0xc0000c6000}, 0x0, {0x6b2568, 0x0}, {0xc0000a6005, 0x0})
	/usr/local/go/src/net/hook.go:23 +0x3d
net.(*Resolver).lookupIPAddr.func1()
	/usr/local/go/src/net/lookup.go:296 +0x9f
internal/singleflight.(*Group).doCall(0x8c2a30, 0xc0000b8050, {0xc0000c8000, 0x1a}, 0xc000028380)
	/usr/local/go/src/internal/singleflight/singleflight.go:95 +0x3b
created by internal/singleflight.(*Group).DoChan
	/usr/local/go/src/internal/singleflight/singleflight.go:88 +0x2f1

goroutine 11 [select]:
net.(*netFD).connect.func2()
	/usr/local/go/src/net/fd_unix.go:119 +0x9e
created by net.(*netFD).connect
	/usr/local/go/src/net/fd_unix.go:118 +0x385

goroutine 22 [runnable]:
net.cgoLookupIP·dwrap·25()
	/usr/local/go/src/net/cgo_unix.go:230
runtime.goexit()
	/usr/local/go/src/runtime/asm_amd64.s:1581 +0x1
created by net.cgoLookupIP
	/usr/local/go/src/net/cgo_unix.go:230 +0x125

