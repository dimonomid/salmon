# Changelog

## 1.0.0 (2026-09-05)


### ⚠ BREAKING CHANGES

* Hide all setup-related commands in salmon-watch under setup
* Hide all setup-related commands under setup
* **systemd:** systemd rules must use `names` instead of `name`. That's fine since there are no users yet other than myself.
* collector YAML configurations must use `conditions` instead of `conds`. This is fine since there are no users yet besides myself.
* YAML and JSON field names have changed without backward compatibility. This is fine because there are no users yet other than myself.
* the item JSON field "comment" is now "details", and the exec YAML field "comment" is now "description". It's ok since there are no users yet other than myself.

### Features

* add guided setup commands for salmon and salmon-watch ([09f31c3](https://github.com/dimonomid/salmon/commit/09f31c3706371b77f82544cbc017a280fb80ca17))
* Add icon to the application entry ([51972f3](https://github.com/dimonomid/salmon/commit/51972f34217bbb5a86b326b3f9545fb2dc5f5061))
* Add logs for pending resolution ([00d8651](https://github.com/dimonomid/salmon/commit/00d8651cdae070eff77e74e2161fd3232dea99b9))
* Add shutdown logs in salmon-watch ([49d9ea2](https://github.com/dimonomid/salmon/commit/49d9ea2cee4faa794e3d0fc03f36b2e725e5446f))
* Add structured application logging ([a10ed29](https://github.com/dimonomid/salmon/commit/a10ed29201f4c8887f89a2d320057a8a2d326d2b))
* Add support for ssh tunnel in salmon-monitor ([0ddfb2d](https://github.com/dimonomid/salmon/commit/0ddfb2d7ad0175647bd500e31e74ec037b4e4abd))
* add TLS support for WebSocket connections ([71a383f](https://github.com/dimonomid/salmon/commit/71a383f701494eab627b2fa7134d2b3aaff56c88))
* **auth:** add bearer token authentication ([4c45cf9](https://github.com/dimonomid/salmon/commit/4c45cf92bd1a7c498800052a361b6618ec2573ec))
* Capture output of exec-ed commands ([b8592eb](https://github.com/dimonomid/salmon/commit/b8592eba3227696244544d66bc655a9e2cd0e9a3))
* Hide all setup-related commands in salmon-watch under setup ([db41c86](https://github.com/dimonomid/salmon/commit/db41c8696d8ffe58140ef26bcae8f0dad7c81359))
* Hide all setup-related commands under setup ([daa0bc6](https://github.com/dimonomid/salmon/commit/daa0bc6a99f837188b15f32dace072f00f06eb97))
* Hide the list of snooze durations behind a Snooze button ([1990bdc](https://github.com/dimonomid/salmon/commit/1990bdc396df4b12f646087121d9bfbe91238a32))
* Implement forgetting stale incidents ([a367638](https://github.com/dimonomid/salmon/commit/a367638ff139197986ceead6c38d34beed0f4fb6))
* Implement timeout for exec commands ([a8430a8](https://github.com/dimonomid/salmon/commit/a8430a8f71b95070d39f55606b002b8aee25f86e))
* Improve icon a little bit ([dc5c581](https://github.com/dimonomid/salmon/commit/dc5c5812381f73210ff20b6d87cffb4c0011b7e1))
* mass rename of misnomers in configs and json protocols ([8602266](https://github.com/dimonomid/salmon/commit/860226642a65607d1cd1ed2f23ef5ea14afd236c))
* rename collector conds to conditions ([dbc5612](https://github.com/dimonomid/salmon/commit/dbc5612e003377f8e46c484271ee03ecf8ff418a))
* rename incident comments to details ([889531b](https://github.com/dimonomid/salmon/commit/889531b29a19a90f7452e36e2baf5466299c6fc1))
* **salmon-watch:** keep status unknown until servers respond ([f8b8a62](https://github.com/dimonomid/salmon/commit/f8b8a622510738802862a30b32c61ef5afe106b0))
* **salmon-watch:** Make setup create application entry as well ([10e3c86](https://github.com/dimonomid/salmon/commit/10e3c860daabff63ea7b8ab083ef0ccb2bd9c3a7))
* **salmon-watch:** show initialization progress in tray icon ([f872e5c](https://github.com/dimonomid/salmon/commit/f872e5ce13beceffee68ce4be8dd5026d6cb2f55))
* **systemd:** allow multiple unit names per rule ([4c9e2f1](https://github.com/dimonomid/salmon/commit/4c9e2f17153aaee2f70f366d712e9896baecddc1))
* **systemd:** detect services stuck in restart loops ([2affb64](https://github.com/dimonomid/salmon/commit/2affb64ec08f825ffbace71c99466fb6b123f453))
* Update incident details with resolution status ([661ca66](https://github.com/dimonomid/salmon/commit/661ca66e32d4a61e7c5781575011088f0878084b))
* use linear reconnect backoff ([5b0f56d](https://github.com/dimonomid/salmon/commit/5b0f56dda26ca85d75a76499d27edce105d0de9e))
* Use salmon user and group for the systemd service ([4e56328](https://github.com/dimonomid/salmon/commit/4e56328b16f56a07c11841256a1d4d478348fbfc))


### Bug Fixes

* apply backpressure instead of dropping state updates ([99e42fb](https://github.com/dimonomid/salmon/commit/99e42fbf3fce91b4d1f855c116afea627a3c5732))
* **core:** make messenger backpressure shutdown-aware ([5f9cd3d](https://github.com/dimonomid/salmon/commit/5f9cd3d758bb5b6f9b722324136072b039083c66))
* harden backend lifecycle and configuration handling ([3cced3b](https://github.com/dimonomid/salmon/commit/3cced3bb6219296a4451f872976410906fbedecd))
* Improve systemd service description ([d0b3520](https://github.com/dimonomid/salmon/commit/d0b352038c0e2abaecbff6aef8b64d34e84bfa48))
* In salmon-watch, add server_id tag to the logs ([5e8a7fa](https://github.com/dimonomid/salmon/commit/5e8a7fa4c2854069254d5b48a74f2a68f274e47c))
* Isolate tunnel subprocess ot a process group ([33a1c87](https://github.com/dimonomid/salmon/commit/33a1c87f065818d2869345b2f3e33f63a0da0b64))
* Make setup commands more cautious ([e57a338](https://github.com/dimonomid/salmon/commit/e57a3385567f1639fc089fd3422edf6401ce03fc))
* **make:** avoid invoking Go for executable suffix ([14c35ba](https://github.com/dimonomid/salmon/commit/14c35ba5809e7a480044bfec04b10274465ea55d))
* reject unexpected positional command arguments ([d04041e](https://github.com/dimonomid/salmon/commit/d04041e160e86add31e4a948e0a9538504380263))
* Remove PrivateTmp from systemd service ([911dd21](https://github.com/dimonomid/salmon/commit/911dd21ef2ae368f9fa9e764ebffa193e2a01339))
* Remove the notification on salmon-watch startup ([c49e714](https://github.com/dimonomid/salmon/commit/c49e7145946cb8a94feb8ba656d58251de26cafe))
* **salmon-watch:** avoid global HTTP server state ([0962c3e](https://github.com/dimonomid/salmon/commit/0962c3e3acd3b674138db39e21771cbf1026c505))
* **salmon-watch:** bind status API to loopback ([2f2c04e](https://github.com/dimonomid/salmon/commit/2f2c04e52e3075265279d79d8d318acb7419d172))
* **salmon-watch:** close status WebSockets on shutdown ([c58f74c](https://github.com/dimonomid/salmon/commit/c58f74c106281b33d211ef5dc6b44758caa33758))
* **salmon-watch:** expire snoozes across system suspend ([32cf30f](https://github.com/dimonomid/salmon/commit/32cf30f6b3a7afbfce2d9b9bb2d446c553dc0a04))
* **salmon-watch:** preserve tray icon flash cadence ([7910de1](https://github.com/dimonomid/salmon/commit/7910de14f91a4d59d505f2b4810b69adbb613b14))
* **salmon-watch:** reject setup on unsupported platforms ([740bb58](https://github.com/dimonomid/salmon/commit/740bb582e14b3bc4d54cfa7c54310f7b1eafc95b))
* **salmon-watch:** stop incident-state worker on close ([c574b77](https://github.com/dimonomid/salmon/commit/c574b77805b08c5c05555b56a204bb41e24686cf))
* **salmon-watch:** validate server IDs ([7c37ef7](https://github.com/dimonomid/salmon/commit/7c37ef7a17faf4187394440c1abd6858eaad59b4))
* Serialize all the wsclient events ([7b59e79](https://github.com/dimonomid/salmon/commit/7b59e79fe3ec3fcce4c9d44a1291aeddca4fec00))
* **service:** always restart Salmon service ([92870e1](https://github.com/dimonomid/salmon/commit/92870e16b4649f915e72eaa2654fd15a5197bdc0))
* **service:** keep retrying after startup failures ([223298c](https://github.com/dimonomid/salmon/commit/223298c9bbdddb7e9b82998ccab424757abd7cfe))
* shell-escape setup command hints ([f75c25e](https://github.com/dimonomid/salmon/commit/f75c25eecd67d11a1661f266cfe967055dac5fb5))
* Shutdown cleanly on Ctrl+C in salmon-watch ([df7f3ec](https://github.com/dimonomid/salmon/commit/df7f3ec66280fc68e2f237eee95ac179281d56b8))
* **statestracker:** publish immutable incident snapshots ([364ba75](https://github.com/dimonomid/salmon/commit/364ba75411a92301e959f7f448dc7b9189fa212c))
* **systemd:** make blocked provider sends interruptible ([f53cef0](https://github.com/dimonomid/salmon/commit/f53cef08018beb670c973f62ea236c10dbaa5e3f))
* Treat not-sent-by-systemd as resolving state in default config ([5ee784a](https://github.com/dimonomid/salmon/commit/5ee784aba9a5d45a2a94f2fcc92a5c6eabc55c70))
* **ui:** preserve open snooze menu across updates ([503a3a4](https://github.com/dimonomid/salmon/commit/503a3a466409a91fd6ea7cbfa2c716fbe1b9b059))
* **watch:** harden WebSocket messages handling. ([b2af958](https://github.com/dimonomid/salmon/commit/b2af958dab55bdaae2f6cfe6fc46d5388bc69e54))
* **webserver:** disconnect slow WebSocket consumers ([c8c86be](https://github.com/dimonomid/salmon/commit/c8c86be4cff9e3d9189656ad95a7089fe8d9afff))
* **wsclient:** apply backpressure to connection events ([2c052a4](https://github.com/dimonomid/salmon/commit/2c052a4252b7164ec2689d077bfc90abd0e108d7))
* **wsclient:** disconnect on malformed server messages ([e4788de](https://github.com/dimonomid/salmon/commit/e4788de19188ecb4d7e67f95e64158e8337d0a50))
