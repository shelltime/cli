# Changelog

## [0.1.46](https://github.com/shelltime/cli/compare/v0.1.45...v0.1.46) (2025-12-27)


### Bug Fixes

* **daemon:** set XDG_RUNTIME_DIR for systemctl --user commands on Linux ([5cba371](https://github.com/shelltime/cli/commit/5cba37131d102c827ad7786e244daa83bb752e78))
* **daemon:** set XDG_RUNTIME_DIR for systemctl --user commands on Linux ([22fa970](https://github.com/shelltime/cli/commit/22fa970aea373cb2c2f4bcf76fac1cbdd5ab260c))
* **model:** add nil check in FindClosestCommand to prevent panic ([7d2ba1d](https://github.com/shelltime/cli/commit/7d2ba1d0b4c7b2ad995ce8ff32573745332d2827))

## [0.1.45](https://github.com/shelltime/cli/compare/v0.1.44...v0.1.45) (2025-12-26)


### Bug Fixes

* **daemon:** fix Linux systemd service template rendering ([3647289](https://github.com/shelltime/cli/commit/3647289c3f8722f9d58ad7885d74eaef9c1d62b3))

## [0.1.44](https://github.com/shelltime/cli/compare/v0.1.43...v0.1.44) (2025-12-25)


### Bug Fixes

* **daemon:** handle bool values sent as strings in OTEL processor ([4134b61](https://github.com/shelltime/cli/commit/4134b618a8fba8f6140eea152a5d926f4c46ff51))
* **daemon:** handle bool values sent as strings in OTEL processor ([1e8fc8f](https://github.com/shelltime/cli/commit/1e8fc8fe8b18cb501f1d382f9191ef8dade45666))

## [0.1.43](https://github.com/shelltime/cli/compare/v0.1.42...v0.1.43) (2025-12-25)


### Features

* **daemon:** add socket-based status query with uptime tracking ([59e5d9a](https://github.com/shelltime/cli/commit/59e5d9aa30db4c21605a80e43fb290c450f0f267))
* **daemon:** add version command with config debug info ([a31d021](https://github.com/shelltime/cli/commit/a31d021173d6f957add7f01bf1dca257be5a7063))
* **daemon:** add version command with config debug info ([e9344c4](https://github.com/shelltime/cli/commit/e9344c49ea8086e545bece6631d2a4ad6a213b8d))
* **daemon:** display CCOtel debug config in status command ([1ace77e](https://github.com/shelltime/cli/commit/1ace77e32929abd8a65a69fb0f2052ffd6a0c970))
* **daemon:** display CCOtel debug config in status command ([2a91532](https://github.com/shelltime/cli/commit/2a9153269eaa6c897d96109bc6163c9defa179d7))


### Bug Fixes

* **daemon:** add debug flag to CCOtel logs debug message ([c7cdfd4](https://github.com/shelltime/cli/commit/c7cdfd4166e4e9de9e07a1d51f1444304071b055))
* **docs:** correct default grpcPort value to 54027 ([3d75da3](https://github.com/shelltime/cli/commit/3d75da3d1948a4f5343d07a306f9bf462141793e))
* **model:** normalize CCOtel debug flag to use consistent truthy variable ([6c5b6a5](https://github.com/shelltime/cli/commit/6c5b6a5af7eef8b6df2c4b66b0854bcb45aa2342))


### Documentation

* **config:** add comprehensive configuration documentation ([e9f1fb7](https://github.com/shelltime/cli/commit/e9f1fb7a41d3577be31173dc71a3b0e494ea6f10))
* **config:** add comprehensive configuration documentation ([5a2d694](https://github.com/shelltime/cli/commit/5a2d694048c9280c7b1285c30816a26a9c0afdcd))


### Miscellaneous Chores

* release 0.1.43 ([1afb762](https://github.com/shelltime/cli/commit/1afb7627557a251a28917009ca7aaa4b910f691a))

## [0.1.42](https://github.com/shelltime/cli/compare/v0.1.41...v0.1.42) (2025-12-25)


### Features

* **ccotel:** add debug config for raw JSON output ([86893b0](https://github.com/shelltime/cli/commit/86893b0ad330aa3979e3464cb577bf9fff3c6b57))
* **ccotel:** add debug config for raw JSON output ([fb5e9b5](https://github.com/shelltime/cli/commit/fb5e9b56945363df6c562e82af264c24c94aa2db))
* **daemon:** add circuit breaker for sync handler ([0e1cc16](https://github.com/shelltime/cli/commit/0e1cc164d6718a61c75cc6d3baa21a8d8a71aeb9))
* **daemon:** add circuit breaker for sync handler ([30dd270](https://github.com/shelltime/cli/commit/30dd270da3da8b027c305e7413a61de7ea21f3c9))
* **daemon:** add coding heartbeat tracking with offline persistence ([f3f61bf](https://github.com/shelltime/cli/commit/f3f61bf41bfd67b43098272ba855a7a8179d2c9e))
* **daemon:** add coding heartbeat tracking with offline persistence ([a65e336](https://github.com/shelltime/cli/commit/a65e3367e0e8f33186f1f4a5318648a81b397e6d))
* **daemon:** add daily cleanup timer service for large log files ([c6d7429](https://github.com/shelltime/cli/commit/c6d742971c6161ce28e577afbc5b612ed053bf6a))
* **daemon:** add daily cleanup timer service for large log files ([723e1f8](https://github.com/shelltime/cli/commit/723e1f8935dda581290aef3966d5cba7ec014e5b))
* **daemon:** add status subcommand ([592954d](https://github.com/shelltime/cli/commit/592954d701a579ad4012cfff7d525b5d8fa66f5f))
* **daemon:** add status subcommand ([0e71436](https://github.com/shelltime/cli/commit/0e71436c6b6acc14eb7d353524a9177f1feb2192))
* **model:** add cache support to ConfigService with skip option ([b86fa51](https://github.com/shelltime/cli/commit/b86fa51d5d9efd5556747d280646f2a456043e77))


### Bug Fixes

* **config:** add missing merge logic for CodeTracking and LogCleanup ([6cbbcb3](https://github.com/shelltime/cli/commit/6cbbcb32917335de448f9072f4f6a79e4bfe27e8))
* **gc:** add nil check when processing post commands ([7aa6099](https://github.com/shelltime/cli/commit/7aa60997bd2b082689c4a10423337019c0f329d4))
* **model:** use template variable for binary path in Linux service file ([957afb9](https://github.com/shelltime/cli/commit/957afb9e43caaba74b9bb7370184ab6f4661c101)), closes [#152](https://github.com/shelltime/cli/issues/152)
* **test:** add LogCleanup to mocked config in track_test ([af725dd](https://github.com/shelltime/cli/commit/af725dda54abf243c70092842e2fc9a0a5fa31af))


### Code Refactoring

* **cli:** rename init command to auth and add new init orchestrator ([5373104](https://github.com/shelltime/cli/commit/5373104c586aea40a0290538bb186a185c3defc8))
* **cli:** rename init command to auth and add new init orchestrator ([cdb6faf](https://github.com/shelltime/cli/commit/cdb6faf024fbd966cb3f8756b211f79d43771b0b))
* **gc:** use path helpers and add automatic large log cleanup ([dc03563](https://github.com/shelltime/cli/commit/dc0356334f16c11f15b7983192f269585db254b7))
* **gc:** use path helpers and add automatic large log cleanup ([0cf2e17](https://github.com/shelltime/cli/commit/0cf2e17c33ba6b20065e5dacb27c73679b30fc2e))
* **logging:** replace logrus with stdlib slog and add path helpers ([03f7962](https://github.com/shelltime/cli/commit/03f7962b72c66f6d257c6ee5dceb28f536d722a7))
* **logging:** replace logrus with stdlib slog and add path helpers ([6144805](https://github.com/shelltime/cli/commit/6144805b4b63527574cfd3a834e034face533b27))
* **model:** move circuit breaker to model package with tests ([bb37156](https://github.com/shelltime/cli/commit/bb37156f57a7ed516195d968138c0ca87246294e))
* **model:** use filepath.Join for cross-platform path handling ([b6e8bf6](https://github.com/shelltime/cli/commit/b6e8bf6f4ed5c045255a8eb931a9c2d2b5043df1))


### Documentation

* **claude:** improve CLAUDE.md with architecture details ([79ffaa9](https://github.com/shelltime/cli/commit/79ffaa9634ea8530b6b3cf476ecfe74bebda94ca))
* **claude:** improve CLAUDE.md with architecture details ([abbca7c](https://github.com/shelltime/cli/commit/abbca7cc75fad78da82085ee26b85eb369cbabd3))


### Miscellaneous Chores

* release 0.1.42 ([7d2fa44](https://github.com/shelltime/cli/commit/7d2fa4442aa79ec46bbeabc9606a514c30e92171))


### Continuous Integration

* **release:** include all commit types in changelog ([a064fbc](https://github.com/shelltime/cli/commit/a064fbc1eed8fa09d60476d146baee5616772a2e))


### Tests

* **config:** add tests for LogCleanup and CodeTracking config ([8055c17](https://github.com/shelltime/cli/commit/8055c177e3bd618a7ce1dc6545754de1b658488f))

## [0.1.41](https://github.com/shelltime/cli/compare/v0.1.40...v0.1.41) (2025-12-19)


### Features

* **ccotel:** add pwd to OTEL_RESOURCE_ATTRIBUTES ([38a5570](https://github.com/shelltime/cli/commit/38a5570ec651395d85a23f8416e5f373c5a827a0))
* **otel:** add pwd (present working directory) to resource attributes ([b718667](https://github.com/shelltime/cli/commit/b71866762f2af3b037a9ae33204e3af937823d99))


### Miscellaneous Chores

* release 0.1.41 ([1197b53](https://github.com/shelltime/cli/commit/1197b535b6102af38fa924fce93e090787d5bf65))

## [0.1.40](https://github.com/shelltime/cli/compare/v0.1.39...v0.1.40) (2025-12-17)


### Bug Fixes

* **otel:** correctly map type attribute based on metric type ([42feaec](https://github.com/shelltime/cli/commit/42feaecf4babc1ed91074d8acb7b0d24d03b6360))

## [0.1.39](https://github.com/shelltime/cli/compare/v0.1.38...v0.1.39) (2025-12-16)


### Bug Fixes

* **otel:** add missing WSL version, prompt, and tool parameters tracking ([67d6b01](https://github.com/shelltime/cli/commit/67d6b01f101200a8033d4cee49d36cb2d644fb5f))

## [0.1.38](https://github.com/shelltime/cli/compare/v0.1.37...v0.1.38) (2025-12-16)


### Features

* **otel:** add complete OTEL configuration fields ([9eaf222](https://github.com/shelltime/cli/commit/9eaf2220de3b5bcc8d927c739c21bf4b997ff862))
* **otel:** add complete OTEL configuration fields to cc install/uninstall ([cda5dce](https://github.com/shelltime/cli/commit/cda5dceada30a4b45a53ca6364ef00f0dd26196d)), closes [#142](https://github.com/shelltime/cli/issues/142)


### Miscellaneous Chores

* release 0.1.38 ([cca2729](https://github.com/shelltime/cli/commit/cca2729846520b2b754f85c6c87daec700eb1db3))

## [0.1.37](https://github.com/shelltime/cli/compare/v0.1.36...v0.1.37) (2025-12-15)


### Features

* **cli:** add cc install command for Claude Code OTEL setup ([cb252ce](https://github.com/shelltime/cli/commit/cb252ce271b08319e6a711dab78dba2426a226ce))
* **daemon:** add Claude Code OTEL v2 passthrough collector ([7b4f728](https://github.com/shelltime/cli/commit/7b4f728720ab7d99e09f3d98d98af712916c1ce2))
* **daemon:** add Claude Code OTEL v2 passthrough collector ([8abfaff](https://github.com/shelltime/cli/commit/8abfaff46deedac40d5678b938d3eed461dbd520))


### Miscellaneous Chores

* release 0.1.37 ([448ad8c](https://github.com/shelltime/cli/commit/448ad8c26e7bea59b74a7e3c4fe700e89aabfbea))

## [0.1.36](https://github.com/shelltime/cli/compare/v0.1.35...v0.1.36) (2025-12-12)


### Features

* **ai:** add configurable tips display via showTips config ([19e37ca](https://github.com/shelltime/cli/commit/19e37ca416810f84faa4c9e9a14458c001a5ea9b))
* **ai:** add configurable tips display via showTips config ([6ae6e25](https://github.com/shelltime/cli/commit/6ae6e25eebe7017402c3ee43c47ca0bae8002dd4))


### Bug Fixes

* **daemon:** base64 encode encrypted track data for JSON transport ([4c85b8b](https://github.com/shelltime/cli/commit/4c85b8b02b9193b146ef5e41c37b51389da16118))
* **daemon:** base64 encode encrypted track data for JSON transport ([6ecf465](https://github.com/shelltime/cli/commit/6ecf4652414ba87a907a378b6df3e0002650a5c6))
* **daemon:** use gui domain instead of deprecated user domain for launchctl ([2381253](https://github.com/shelltime/cli/commit/2381253999ca7e5c40c2863ab0d1362aa8c4fe33))
* **daemon:** use gui domain instead of deprecated user domain for launchctl ([c5e132e](https://github.com/shelltime/cli/commit/c5e132eecd7c474d632c3ff997e47ecea5e85215))


### Miscellaneous Chores

* release 0.1.36 ([a30e94b](https://github.com/shelltime/cli/commit/a30e94be047c63ce376d6f3705936e31e9f01470))

## [0.1.35](https://github.com/shelltime/cli/compare/v0.1.34...v0.1.35) (2025-10-06)


### Bug Fixes

* **daemon:** replace hardcoded username with $USER in systemd service ([2610348](https://github.com/shelltime/cli/commit/2610348aa18be701c2dbb2bb228d81fe10343486))
* **daemon:** replace hardcoded username with $USER in systemd service ([964538c](https://github.com/shelltime/cli/commit/964538c7c6ebc90c2f024e163edf39e85fe05c00))

## [0.1.34](https://github.com/shelltime/cli/compare/v0.1.33...v0.1.34) (2025-10-05)


### Features

* **daemon:** update service descriptors for systemd and launchctl ([356f6d1](https://github.com/shelltime/cli/commit/356f6d1069ed7686d7c5a82b419f692eb19d3421))
* **daemon:** update service descriptors for systemd and launchctl ([ea63e31](https://github.com/shelltime/cli/commit/ea63e31dc475a259031de0036a08221e9c35ab85))


### Miscellaneous Chores

* release 0.1.34 ([6206d21](https://github.com/shelltime/cli/commit/6206d21e996dcaaa2b7dee06b04d24bdfedd93f5))

## [0.1.33](https://github.com/shelltime/cli/compare/v0.1.32...v0.1.33) (2025-10-05)


### Bug Fixes

* **model:** use user shell for ccusage command execution ([a3a0d96](https://github.com/shelltime/cli/commit/a3a0d96a85c92df98e7bfde960e6c7f242c32303))
* **model:** use user shell for ccusage command execution ([3212ab9](https://github.com/shelltime/cli/commit/3212ab968388c9fed82e619c5a3b07e4352929c7))

## [0.1.32](https://github.com/shelltime/cli/compare/v0.1.31...v0.1.32) (2025-10-05)


### Bug Fixes

* **commands:** resolve query command test flakiness ([f7bf69d](https://github.com/shelltime/cli/commit/f7bf69d3d4dcf74f9485ab908ec0c9a7c1ddba92))
* **commands:** update test assertions for auto-run commands ([781a832](https://github.com/shelltime/cli/commit/781a83237d4a519b46450439ed792e904b9d01e1))
* **model:** add --yes flag to npx ccusage command ([971c6e6](https://github.com/shelltime/cli/commit/971c6e6ea913a691f41a6d8bfab092ce4a28f6c1))
* **model:** add --yes flag to npx ccusage command ([88f2d49](https://github.com/shelltime/cli/commit/88f2d49b9f93a184a58ff51a61ea8293ad74acca)), closes [#125](https://github.com/shelltime/cli/issues/125)

## [0.1.31](https://github.com/shelltime/cli/compare/v0.1.30...v0.1.31) (2025-10-04)


### Bug Fixes

* **commands:** resolve flaky track test assertion ([0da55b7](https://github.com/shelltime/cli/commit/0da55b78f4d1068fcfad1c8b1535936e95dcd9d5))

## [0.1.30](https://github.com/shelltime/cli/compare/v0.1.29...v0.1.30) (2025-10-04)


### Features

* **daemon:** support user-scope systemd service installation on Linux ([3b1b2a5](https://github.com/shelltime/cli/commit/3b1b2a5473e8a19a5a17e037f64c1ccbfa9f11b1))
* **daemon:** support user-scope systemd service installation on Linux ([c1aaa9f](https://github.com/shelltime/cli/commit/c1aaa9f5c8b2c6221e4ce61765fd64cbe554b37d))
* **model:** remove msgpack support and replace with JSON ([6a8229e](https://github.com/shelltime/cli/commit/6a8229ec11d2d7d55552dae24dddd00493f8cb94))
* remove msgpack support and replace with JSON ([64a87e7](https://github.com/shelltime/cli/commit/64a87e7757259db45fa6b7ca64c616b9fba99097))


### Bug Fixes

* **model:** enhance lookPath with shell which command fallback ([46bf6a3](https://github.com/shelltime/cli/commit/46bf6a3a2abe186a7599e3478c9962f084d11d69))


### Miscellaneous Chores

* release 0.1.30 ([0a2b6bb](https://github.com/shelltime/cli/commit/0a2b6bb36f1e685d074d181d04fbac192bb5e411))

## [0.1.29](https://github.com/shelltime/cli/compare/v0.1.28...v0.1.29) (2025-10-02)


### Bug Fixes

* **handshake:** update Content-Type header to application/json ([d0875ae](https://github.com/shelltime/cli/commit/d0875ae2caefbd89f4fc2d8236ff2d49171e5c56))
* **model:** add fnm support to custom lookPath ([7861724](https://github.com/shelltime/cli/commit/78617246fb9a609c9b1a39c0c6438eb14ced0014))
* **model:** implement custom lookPath for node package managers ([63020ad](https://github.com/shelltime/cli/commit/63020ad3fb5ba1a238f09400ec96a5c01e700eca))
* **model:** implement custom lookPath for node package managers ([dd10653](https://github.com/shelltime/cli/commit/dd10653fef8dcb5b7f73fa8b5b602c9deafe50cc)), closes [#112](https://github.com/shelltime/cli/issues/112)

## [0.1.28](https://github.com/shelltime/cli/compare/v0.1.27...v0.1.28) (2025-10-01)


### Features

* **daemon:** move macOS daemon install from root to user permissions ([b9d7625](https://github.com/shelltime/cli/commit/b9d7625383eba600e9cb3d778091458dfe88651b))
* **daemon:** move macOS daemon install from root to user permissions ([dc759c8](https://github.com/shelltime/cli/commit/dc759c82a2d4b811a6a2e2033ad1d615018621a6))


### Bug Fixes

* **daemon:** remove unused import and fix formatting in uninstall command ([a0279d9](https://github.com/shelltime/cli/commit/a0279d9a42dcdc5e19ec3b620ba0897fe43f09a1))


### Miscellaneous Chores

* release 0.1.28 ([552f43a](https://github.com/shelltime/cli/commit/552f43ab4d174f48edbc1324abf8234a5b949f85))

## [0.1.27](https://github.com/shelltime/cli/compare/v0.1.26...v0.1.27) (2025-09-28)


### ⚠ BREAKING CHANGES

* **model:** Renamed SendHTTPRequest to SendHTTPRequestJSON to reflect the change in default serialization format from msgpack to JSON. This affects all API calls using the HTTP request function.

### Features

* **model:** change default HTTP send method from msgpack to JSON ([1a09822](https://github.com/shelltime/cli/commit/1a098222ba93a8194459e2d8a4cc95d18ac87975))


### Bug Fixes

* **model:** update api tests to use JSON instead of msgpack ([eca15ab](https://github.com/shelltime/cli/commit/eca15abbc4ef658afda388116f8d3a7e686490b7))
* **test:** update tests to use JSON instead of msgpack ([8bb6ad0](https://github.com/shelltime/cli/commit/8bb6ad0703dc1e9f4c58678e6e156cd195466e33))


### Miscellaneous Chores

* release 0.1.27 ([125465f](https://github.com/shelltime/cli/commit/125465f0a6249004851945846428948c0df97807))

## [0.1.26](https://github.com/shelltime/cli/compare/v0.1.25...v0.1.26) (2025-09-27)


### Features

* add daemon reinstall command ([720f195](https://github.com/shelltime/cli/commit/720f1957ca1e1b497ae7fb9ffb55de25c0f13241)), closes [#97](https://github.com/shelltime/cli/issues/97)
* **api:** add SendHTTPRequestJSON and refactor GraphQL to use it ([773aaaf](https://github.com/shelltime/cli/commit/773aaaf5ca3ce4cdfa0236ba12e3b162d10a6b8b))
* **ccusage:** add CCUsage collection service for usage tracking ([d930d8e](https://github.com/shelltime/cli/commit/d930d8e04b4be39a4cece7627ff854b166946681))
* **ccusage:** add CCUsage collection service for usage tracking ([9e9bf18](https://github.com/shelltime/cli/commit/9e9bf181791e57e7110f4a345bc88e1bcf084cb6))
* **ccusage:** add incremental sync support with since parameter ([3a86e36](https://github.com/shelltime/cli/commit/3a86e36a78879b837faf54dfe2d92f8695ce08f1))
* **config:** add exclude patterns to filter commands from server sync ([d8e79e7](https://github.com/shelltime/cli/commit/d8e79e7bed25629c88086505d4257a2da1a1121e))
* **dotfile:** add advanced merge support for config files ([eaf7ea2](https://github.com/shelltime/cli/commit/eaf7ea2ec7e2b02e8d9d73becd00a2df8a020e83))
* **dotfile:** add advanced merge support for Ghostty config files ([f32fd42](https://github.com/shelltime/cli/commit/f32fd4219ca4df6ffbcf01c5b5449811e89bbe57))
* **dotfile:** add support for ignoring sections in collected files ([a8332ee](https://github.com/shelltime/cli/commit/a8332ee086426a7fdf2e4417217d1964c9ac7ad9))
* **dotfile:** add support for ignoring sections in collected files ([2d7b9af](https://github.com/shelltime/cli/commit/2d7b9af6c9d94d3cdd16fd3fe6237632a2129c20))


### Bug Fixes

* **ccusage:** handle timestamp format from GraphQL API ([821150e](https://github.com/shelltime/cli/commit/821150ef2ad2a3ecd67ea8ba3678b9d52205b0c7))
* **ccusage:** improve debug log formatting for timestamps ([13015d8](https://github.com/shelltime/cli/commit/13015d839bd5fa7bb0161c9eae9b6e3c6991902d))
* **ccusage:** update GraphQL query to use correct schema and filter by hostname ([5017fe4](https://github.com/shelltime/cli/commit/5017fe4f1a04d49347218be81ac79a41b630d2d7))


### Miscellaneous Chores

* release 0.1.26 ([2d3ca5e](https://github.com/shelltime/cli/commit/2d3ca5e1e67228970f52ebe0fc0dd46f675f6d9a))

## [0.1.25](https://github.com/shelltime/cli/compare/v0.1.24...v0.1.25) (2025-09-06)


### Features

* **dotfiles:** add dry-run mode and experimental diff merge service ([3a35d1d](https://github.com/shelltime/cli/commit/3a35d1db1c5a3415eb8d5077fbab8f8e2e7d6c03))
* **dotfiles:** add pterm for beautiful pull command output ([c696d5f](https://github.com/shelltime/cli/commit/c696d5fb5ed78df3034c5d49760c77f1c808995c))
* **dotfiles:** enhance pull command with detailed per-app file status tracking ([dd47cc9](https://github.com/shelltime/cli/commit/dd47cc996bc5b111201b19ba80531e48d2457fea))
* **dotfiles:** enhance pull command with detailed per-app file status tracking ([926b4a3](https://github.com/shelltime/cli/commit/926b4a3610416ce14c93f0aaf1ad796cdb3c831e))
* improve dotfiles pull file-saving with go-diff ([480a872](https://github.com/shelltime/cli/commit/480a872a38ace9d05fad5205988e8ae8a1744f24))
* improve dotfiles pull file-saving with go-diff ([49472ee](https://github.com/shelltime/cli/commit/49472ee8f599d290545b97b058d1b5083779c241))


### Bug Fixes

* **model:** improve diff trim to remove additional control characters ([63ebf68](https://github.com/shelltime/cli/commit/63ebf686dd7f820c09c1fed73f54929639df0caa))
* **model:** split ApplyDiff into GetChanges and ApplyDiff methods ([e3f9ecf](https://github.com/shelltime/cli/commit/e3f9ecfafa4d7fd2024a10d40fac672131715d54))
* **model:** update dotfile_apps to use new DiffMergeService API ([5f69531](https://github.com/shelltime/cli/commit/5f695313e4098d8446ae1f1eb5ed654cc532c29b))
* use merged content from go-diff patches instead of just newContent ([aab6b9b](https://github.com/shelltime/cli/commit/aab6b9b6a283318f533498bf9e3f989110d2e62e))


### Miscellaneous Chores

* release 0.1.25 ([5fae10e](https://github.com/shelltime/cli/commit/5fae10e2dfda6cd20a9317015cf6227c3106a4e1))

## [0.1.24](https://github.com/shelltime/cli/compare/v0.1.23...v0.1.24) (2025-09-03)


### Features

* add npm, ssh, kitty, and kubernetes dotfile support ([32c39a2](https://github.com/shelltime/cli/commit/32c39a2023631b307145fd8f63b16038b095051c))
* add npm, ssh, kitty, and kubernetes dotfile support ([3bc5cb0](https://github.com/shelltime/cli/commit/3bc5cb0afb6aa05d4c2cf308a96e817c7f23ed3e))
* add starship config support to dotfiles push/pull ([f7a7d1a](https://github.com/shelltime/cli/commit/f7a7d1a5543acbcae4238c19e720a9c9a9d8ac99))
* add starship config support to dotfiles push/pull ([06e4104](https://github.com/shelltime/cli/commit/06e4104ed19a59782648229ee11d29be04784a1d))


### Miscellaneous Chores

* release 0.1.24 ([83ef433](https://github.com/shelltime/cli/commit/83ef433720e77881a6ff4f5ce12f5832a4126966))

## [0.1.23](https://github.com/shelltime/cli/compare/v0.1.22...v0.1.23) (2025-09-01)


### Features

* add claude configuration support to dotfiles push/pull ([90515c4](https://github.com/shelltime/cli/commit/90515c4343ae00347a0b31b16d4d9344a879cfab))
* add claude configuration support to dotfiles push/pull ([63a67f0](https://github.com/shelltime/cli/commit/63a67f0fc71e85500cc0b79bb82e50e937dbabe5)), closes [#84](https://github.com/shelltime/cli/issues/84)


### Bug Fixes

* **dotfiles:** update Claude config paths to match actual usage ([e738275](https://github.com/shelltime/cli/commit/e73827536b02b389d5005775c417664c0786bc91))


### Miscellaneous Chores

* release 0.1.23 ([124c950](https://github.com/shelltime/cli/commit/124c9502dcd8ffefdf543c413b2851286f69974e))

## [0.1.22](https://github.com/shelltime/cli/compare/v0.1.21...v0.1.22) (2025-08-31)


### Features

* **cli:** add dotfiles command for managing configuration files ([d64d828](https://github.com/shelltime/cli/commit/d64d82856b16c2c85ebcc7e58c06ed4244af6da6))
* **cli:** add dotfiles command for managing configuration files ([4926495](https://github.com/shelltime/cli/commit/49264955f8e33be4394a391e09a183e2ef328d3c))
* **config:** support local config file override ([8613ffd](https://github.com/shelltime/cli/commit/8613ffd5d0014e4d5193b038b68b1ba65bd80439))
* **dotfiles:** add pull command to sync dotfiles from server ([ba17256](https://github.com/shelltime/cli/commit/ba172563144d295fc6fcc538c63c912816ceca03))


### Bug Fixes

* **dotfiles:** update GraphQL endpoint path for dotfiles API ([aa7084e](https://github.com/shelltime/cli/commit/aa7084e275166f94e06ec87f8706e3ad070f879e))


### Miscellaneous Chores

* release 0.1.22 ([32e7cd2](https://github.com/shelltime/cli/commit/32e7cd25a4295196b076afcf369893f678355475))

## [0.1.21](https://github.com/shelltime/cli/compare/v0.1.20...v0.1.21) (2025-07-25)


### Bug Fixes

* **cli:** update copyright year to 2025 ([f7648e4](https://github.com/shelltime/cli/commit/f7648e4ec877c3a35c1b6021d572d76f2ff3c0ce))

## [0.1.20](https://github.com/shelltime/cli/compare/v0.1.19...v0.1.20) (2025-07-25)


### Bug Fixes

* **gitignore:** add promptpal binary to gitignore ([bca12eb](https://github.com/shelltime/cli/commit/bca12ebc536ee6fb6d9698a910e650afad66b6cc))

## [0.1.19](https://github.com/shelltime/cli/compare/v0.1.18...v0.1.19) (2025-07-25)


### Features

* add AI query feature support ([cbd6751](https://github.com/shelltime/cli/commit/cbd67516ca5922c125775e78bbf63fa8d625ec53))
* add msgpack encode/decode module ([fa1c8bb](https://github.com/shelltime/cli/commit/fa1c8bb53b196cc7ffffd5a2254dce30c270d0f7))
* add query command with AI integration ([a84869a](https://github.com/shelltime/cli/commit/a84869a770a9d03e38653fc78ced057c0cae41ac))
* **ai:** add AI auto-run configuration and command classification ([8787fbb](https://github.com/shelltime/cli/commit/8787fbbd203dd37db0ba405803e1f5eb6504eec1))
* **ai:** add PromptPal CI/CD integration ([96a9bda](https://github.com/shelltime/cli/commit/96a9bda9cf6b3bb09308b164db5f96e9e68ba339))
* **ai:** use user token from config file for AI service ([eb7fc7b](https://github.com/shelltime/cli/commit/eb7fc7ba47914df069c62266a582dbdc74f2c37c))
* **daemon:** add PromptPal configuration variables ([7647827](https://github.com/shelltime/cli/commit/7647827f5a604d419874baf0f065a8d7f89a3d57))


### Bug Fixes

* **ci:** use production PromptPal token in release workflow ([18f2f94](https://github.com/shelltime/cli/commit/18f2f949b5d7018480a45a2231f7e52c508e6ad2))
* **config:** remove extra quotes from PromptPal token configuration ([aed01ec](https://github.com/shelltime/cli/commit/aed01ec4d857a6bf7ccd183593ea5ed58ee482d7))
* remove .windsurfrules file ([350a1ab](https://github.com/shelltime/cli/commit/350a1ab18097234071fc111697857accd9e4a3b1))


### Miscellaneous Chores

* release 0.1.19 ([034b856](https://github.com/shelltime/cli/commit/034b85679507f58b2cc6311521bafc7c98b2c7ae))

## [0.1.18](https://github.com/shelltime/cli/compare/v0.1.17...v0.1.18) (2025-06-30)


### Bug Fixes

* **docs:** add CLAUDE.md for AI-assisted development ([6eb7de4](https://github.com/shelltime/cli/commit/6eb7de4394c6fade0b5f3eacc3d6645d336fc79c))
* update release workflow and go.mod dependencies ([6d44100](https://github.com/shelltime/cli/commit/6d44100a5a5418136fc11774440465907185b0a2))
* update test files and configuration ([d3ef2be](https://github.com/shelltime/cli/commit/d3ef2be78e59b2e342f9b8ce9d9b861f4bd636c3))

## [0.1.17](https://github.com/shelltime/cli/compare/v0.1.16...v0.1.17) (2025-05-31)


### Features

* **bash:** add bash shell hook support with install/uninstall functionality ([2af5d5f](https://github.com/shelltime/cli/commit/2af5d5fff6f2c15f2dc4131725c3903fd61b45c5))
* **bash:** add hook file existence check before bash shell installation ([8a33f80](https://github.com/shelltime/cli/commit/8a33f80fef40a0e3891601499e54019db69f5d86))
* **cli:** add doctor command and shell hook installation functionality ([504720b](https://github.com/shelltime/cli/commit/504720b421d5909198f80c6f9a4b69723de8f4e3))


### Bug Fixes

* **doctor:** add daemon service check and improve path handling with base folder validation ([6ca63c5](https://github.com/shelltime/cli/commit/6ca63c541aa8ee7e8dba6bf24262d7ebebb84397))
* **install:** add bin folder check and GitHub issues permission for release workflow ([16feced](https://github.com/shelltime/cli/commit/16fecedb4cf2f0f2afe86228df650958d6407d10))


### Miscellaneous Chores

* release 0.1.17 ([a581bc7](https://github.com/shelltime/cli/commit/a581bc7aa087f9ad4442bddfa0a48d2abedc8437))

## [0.1.16](https://github.com/shelltime/cli/compare/v0.1.15...v0.1.16) (2025-03-04)


### Bug Fixes

* **ci:** rebrand to shelltime ([cbf61eb](https://github.com/shelltime/cli/commit/cbf61ebbeb7ec6ad98a9aa467c69bd1b5c20de39))

## [0.1.15](https://github.com/shelltime/cli/compare/v0.1.14...v0.1.15) (2025-03-04)


### Features

* **alias:** add alias import cli ([3a02c33](https://github.com/shelltime/cli/commit/3a02c33c41a9801be785341f91d1287bacd9f4cd))


### Bug Fixes

* **alias:** add alias import command ([114ec91](https://github.com/shelltime/cli/commit/114ec9120950e36a1d73e469c36eabf296c38f4c))


### Miscellaneous Chores

* release 0.1.15 ([4c74f13](https://github.com/shelltime/cli/commit/4c74f134b84b7d009894fcc548623b4ff1c5822c))

## [0.1.14](https://github.com/malamtime/cli/compare/v0.1.13...v0.1.14) (2025-01-24)


### Bug Fixes

* **daemon:** nack if message not handled ([183a226](https://github.com/malamtime/cli/commit/183a226db2fbfa5ee6dc46bafa4acaebdb493b4d))

## [0.1.13](https://github.com/malamtime/cli/compare/v0.1.12...v0.1.13) (2025-01-24)


### Bug Fixes

* **daemon:** do not return if handle error ([6ae2979](https://github.com/malamtime/cli/commit/6ae29790124f452fe0e602a3f384f105c3a6c8f6))
* **docs:** add encryption on readme.md ([e89d595](https://github.com/malamtime/cli/commit/e89d5959541fd0b2b885124b4430a01ebe5a51ce))

## [0.1.12](https://github.com/malamtime/cli/compare/v0.1.11...v0.1.12) (2025-01-20)


### Features

* **daemon:** add encryption tests on daemon service ([18520ed](https://github.com/malamtime/cli/commit/18520ed6f6fc42c62d5fd00ac6c59217f3191ed1))
* **track:** add encrypt mode ([9135d80](https://github.com/malamtime/cli/commit/9135d807642cb73deafd421e45cd496a4d96fc20))


### Bug Fixes

* **command:** add source field whcih is daemon or cli ([0b645ba](https://github.com/malamtime/cli/commit/0b645ba0cb1c256ff3eb3c63205315bc009a1507))


### Miscellaneous Chores

* release 0.1.12 ([2c93aad](https://github.com/malamtime/cli/commit/2c93aad1f861d9cb63c396118909ebfcf87f2e3c))

## [0.1.11](https://github.com/malamtime/cli/compare/v0.1.10...v0.1.11) (2025-01-12)


### Bug Fixes

* add uninstall daemon service ([594c1b9](https://github.com/malamtime/cli/commit/594c1b9794c66cb14cd087aff1e353b06b3a0d46))
* **doc:** add daemon section ([29aa98c](https://github.com/malamtime/cli/commit/29aa98c620ee84b1d64e7aac8577fc02444b64d9))
* **docs:** add info about daemon service ([53dc12f](https://github.com/malamtime/cli/commit/53dc12f7f83f273b1f52ab9fb08ba10ff1d440d7))

## [0.1.10](https://github.com/malamtime/cli/compare/v0.1.9...v0.1.10) (2025-01-04)


### Bug Fixes

* **cli:** add `ls` for read local data ([a96650c](https://github.com/malamtime/cli/commit/a96650c530c07ddf90ea03e169b4a0e6ecb1aa7b))
* **cli:** add `web` command for open web portal in cli ([7724634](https://github.com/malamtime/cli/commit/7724634995dacba5f6ba49997e0accf5061472f8))
* **cli:** fix daemon help info ([68e0c82](https://github.com/malamtime/cli/commit/68e0c82c7e62cf9e41067c1fb1b477d55af9c333))
* **hooks:** add hooks uninstall method ([499a997](https://github.com/malamtime/cli/commit/499a99725ed5d1fb2e7bc8baa265f01fbb8af01a))
* **ls:** add more warning message for `ls` command ([8a95484](https://github.com/malamtime/cli/commit/8a95484c4577bf84d236d105097df11da3b47309))
* **track:** support nano time in command ([adaa15a](https://github.com/malamtime/cli/commit/adaa15a41d0d8c5cde8ebb076bb7d40ac68a5f77))

## [0.1.9](https://github.com/malamtime/cli/compare/v0.1.8...v0.1.9) (2025-01-01)


### Bug Fixes

* **daemon:** fix symbol link detech on looking username in folder ([528875a](https://github.com/malamtime/cli/commit/528875a0ebe234f1f5b8428d2cddddf70fa21f36))

## [0.1.8](https://github.com/malamtime/cli/compare/v0.1.7...v0.1.8) (2024-12-30)


### Bug Fixes

* **daemon:** fix version ([106234c](https://github.com/malamtime/cli/commit/106234c3204b9a90eae30f6dc5e063946bbcdc9c))
* **daemon:** use root to run daemon service in linux ([c49bfcf](https://github.com/malamtime/cli/commit/c49bfcfaaec169a0429758b1b9d83b58e93c3be2))

## [0.1.7](https://github.com/malamtime/cli/compare/v0.1.6...v0.1.7) (2024-12-29)


### Bug Fixes

* **daemon:** change the socket permission to 777 ([08a1833](https://github.com/malamtime/cli/commit/08a18334af12b4ba4078e2625d025a1e30f94319))
* **daemon:** fix default daemon config issue ([ec5dd76](https://github.com/malamtime/cli/commit/ec5dd769ead47020122d67a2b4280bf31e602123))

## [0.1.6](https://github.com/malamtime/cli/compare/v0.1.5...v0.1.6) (2024-12-29)


### Bug Fixes

* **daemon:** fix plist in daemon ([de83453](https://github.com/malamtime/cli/commit/de834537f91c67b989e4f6c843c92400352a998f))

## [0.1.5](https://github.com/malamtime/cli/compare/v0.1.4...v0.1.5) (2024-12-29)


### Bug Fixes

* **daemon:** install the daemon service in user level ([2507c34](https://github.com/malamtime/cli/commit/2507c34487e9360e6454346262517c1ac5dd53aa))

## [0.1.4](https://github.com/malamtime/cli/compare/v0.1.3...v0.1.4) (2024-12-29)


### Bug Fixes

* **cli:** fix command level of daemon install in cli ([aabc9eb](https://github.com/malamtime/cli/commit/aabc9eb6b721de339e15f9fa7b1140aa0e99e72d))
* **daemon:** change the permission on socket created ([a02c863](https://github.com/malamtime/cli/commit/a02c863e05074adbf9e3094af8f5cef4f9a3ba48))

## [0.1.3](https://github.com/malamtime/cli/compare/v0.1.2...v0.1.3) (2024-12-29)


### Bug Fixes

* **ci:** fix ci to allow multiple binary to archive ([40dd798](https://github.com/malamtime/cli/commit/40dd798bcc0ebd63d23bd38a97ee35e5bd9cd5eb))

## [0.1.2](https://github.com/malamtime/cli/compare/v0.1.1...v0.1.2) (2024-12-29)


### Bug Fixes

* **ci:** move the daemon into a single pack ([5146452](https://github.com/malamtime/cli/commit/51464529d89116888a50c7f056739f1252439699))

## [0.1.1](https://github.com/malamtime/cli/compare/v0.1.0...v0.1.1) (2024-12-29)


### Bug Fixes

* **daemon:** add otel ([4d1c167](https://github.com/malamtime/cli/commit/4d1c167e7ec903eee62a41cf4ddb5c8bf4e33fbc))

## [0.1.0](https://github.com/malamtime/cli/compare/v0.0.49...v0.1.0) (2024-12-29)


### ⚠ BREAKING CHANGES

* daemon process arrived!

### Features

* add daemon service for no wait synchronization ([854384c](https://github.com/malamtime/cli/commit/854384c631c85c440d7342d1c9d0ed7fcaa1a143))
* **cli:** add uninstall command for daemon service ([7166543](https://github.com/malamtime/cli/commit/7166543f9bb495e3b474901269c6d447ef8aa42d))
* **cli:** send to socket if the socket is ready. send to http if not ([9d294c8](https://github.com/malamtime/cli/commit/9d294c826a5db3f0ef8d841ca469c74e9882bba8))
* **daemon:** add basic socket and event handler for daemon ([c82aba3](https://github.com/malamtime/cli/commit/c82aba3396836ef8fc92cdcc2eb397aff1614e14))
* **daemon:** add tests for daemon call ([fa4374c](https://github.com/malamtime/cli/commit/fa4374ce9b4450a8da0bedbb5ab720b96526b017))


### Bug Fixes

* **cli:** add daemon install command ([24ec1bc](https://github.com/malamtime/cli/commit/24ec1bc6565037c10645dc40801383ebad7acd87))
* **cli:** add dry-run flag on sync ([7aaebe9](https://github.com/malamtime/cli/commit/7aaebe984a4695a15c5c23ca8d61473bb33dcfe0))
* **cli:** finish daemon installation basiclly ([f9a26c6](https://github.com/malamtime/cli/commit/f9a26c6497c6081f530bcbba43a07e3b0963503a))
* **cli:** update daemon install cmd register ([2b18297](https://github.com/malamtime/cli/commit/2b182975a2dfed5b30ecec21fe2a588ecbbf673d))
* **daemon:** add daemon service checking and syncing methods ([3638ec6](https://github.com/malamtime/cli/commit/3638ec6c31ec0a24a52d55dfab79435215f8b104))
* **daemon:** add pubsub close on process end ([f1ef90b](https://github.com/malamtime/cli/commit/f1ef90b8dfcd62777049adf8bffa56ffc8a2f695))
* **daemon:** fix daemon parser ([45550bc](https://github.com/malamtime/cli/commit/45550bc78e4ad3e07037db719d30ad63696cdb69))
* **daemon:** fix daemon test file ([b5c8a5d](https://github.com/malamtime/cli/commit/b5c8a5d70f79249021c374318cb2e750ea274f6b))
* **daemon:** fix daemon tests ([35e2d72](https://github.com/malamtime/cli/commit/35e2d72006f71af362769f6816ae723dc0ead0bb))
* **daemon:** fix test file ([70f1c15](https://github.com/malamtime/cli/commit/70f1c1550f3ed0bc950b3516eb2d94ce17a079f4))
* **daemon:** fix test files for daemon ([0472175](https://github.com/malamtime/cli/commit/04721756867385635267a6c5f1de56af3566e074))
* **daemon:** update daemon client ([f3e07f6](https://github.com/malamtime/cli/commit/f3e07f68f0e35aa64213069bae4d44d38ce383f4))
* **handshake:** fix testcase for handshake ([f30ac51](https://github.com/malamtime/cli/commit/f30ac51bd86ca8df35cdf16dfb86608c188341d4))
* **track:** add log for backend is working ([67705b1](https://github.com/malamtime/cli/commit/67705b10f9f6e3afc1967a7706876bd4f1d793c2))


### Miscellaneous Chores

* release 0.1.0 ([80304fe](https://github.com/malamtime/cli/commit/80304fecf0e69c3f6de3eb41c789057fdcdc0407))

## [0.0.49](https://github.com/malamtime/cli/compare/v0.0.48...v0.0.49) (2024-12-13)


### Bug Fixes

* **track:** trim content before checking ([980f986](https://github.com/malamtime/cli/commit/980f98665bba71c48298f51e7b7103d84949633c))

## [0.0.48](https://github.com/malamtime/cli/compare/v0.0.47...v0.0.48) (2024-12-13)


### Bug Fixes

* **docs:** add performance explaination ([a467327](https://github.com/malamtime/cli/commit/a46732786e0f522f0cf08fd8ef17475040cc57c9))
* **docs:** fix config field in readme ([70ebee0](https://github.com/malamtime/cli/commit/70ebee03c944ce25533732ff94d97db963746c9c))
* **docs:** fix docs ([5473019](https://github.com/malamtime/cli/commit/547301994db88fb4f9a5a6c324d905698c019abc))
* **track:** increase the buffer length when parse file line by line ([6145772](https://github.com/malamtime/cli/commit/6145772df0fbf46cb317f8710d9f005630dc1b1c))

## [0.0.47](https://github.com/malamtime/cli/compare/v0.0.46...v0.0.47) (2024-12-13)


### Bug Fixes

* **sync:** make the sync could be force in `sync` command ([025ab3a](https://github.com/malamtime/cli/commit/025ab3a221c576f2ae8822d0211690d58ce6df7d))

## [0.0.46](https://github.com/malamtime/cli/compare/v0.0.45...v0.0.46) (2024-12-13)


### Features

* **sync:** add docs about `sync` command ([826f316](https://github.com/malamtime/cli/commit/826f31668dfc7140def0be123593d863964b3f36))
* **sync:** add sync command ([2236979](https://github.com/malamtime/cli/commit/2236979b0bbbc82a4a044db801e79a9307339a5e))


### Bug Fixes

* **docs:** remove unused docs ([a3e77fc](https://github.com/malamtime/cli/commit/a3e77fc2de237421a8cdc7571814b697c815e6c4))


### Miscellaneous Chores

* release 0.0.46 ([cc44364](https://github.com/malamtime/cli/commit/cc443646677dc7856a4b51c7a85e124de2a52d3c))

## [0.0.45](https://github.com/malamtime/cli/compare/v0.0.44...v0.0.45) (2024-12-13)


### Bug Fixes

* **docs:** update readme ([8c0f281](https://github.com/malamtime/cli/commit/8c0f281784c23afe273ed886ad9028e4d3bee48d))
* **gc:** remove unused empty line on gc ([eea8312](https://github.com/malamtime/cli/commit/eea8312943f629705ac50402ddbe463470c7ce64))


### Performance Improvements

* **model:** `GetPreCommandsTree` performance improve and add benchmark tests ([a0f8ae8](https://github.com/malamtime/cli/commit/a0f8ae8f6f4fb743e179c44b0eefc315063e6609))
* **model:** improve performance on `GetPreCommands` ([8d83ce2](https://github.com/malamtime/cli/commit/8d83ce26344f5645d1022722c37806ef3195e8b3))
* **model:** use bytes operators on postFile to improve performance ([6d15ca0](https://github.com/malamtime/cli/commit/6d15ca0d2794381cccb9f6625c1951cca64fe48d))

## [0.0.44](https://github.com/malamtime/cli/compare/v0.0.43...v0.0.44) (2024-12-13)


### Bug Fixes

* **ci:** ignore codecov generated files ([2109547](https://github.com/malamtime/cli/commit/21095477f36dcd357ad2548031e290ed92158f56))

## [0.0.43](https://github.com/malamtime/cli/compare/v0.0.42...v0.0.43) (2024-12-13)


### Bug Fixes

* **ci:** add uptrace on ci ([cac2a2e](https://github.com/malamtime/cli/commit/cac2a2e3a3cde61cad7497af89fa713ef8c77d38))
* **ci:** set timeout as 3m ([91ca363](https://github.com/malamtime/cli/commit/91ca3634747d6069f5a4a4f9488c7e319cc6fc89))
* **ci:** upgrade codecov action to v5 ([d02c5d6](https://github.com/malamtime/cli/commit/d02c5d6b260ecad2e66def25886e8ccc703d040b))
* **track:** fix mock config service ([bb944f9](https://github.com/malamtime/cli/commit/bb944f9396245d3976c76970708b1800dcc3c290))

## [0.0.42](https://github.com/malamtime/cli/compare/v0.0.41...v0.0.42) (2024-12-13)


### Features

* **trace:** add trace for cli ([bea74de](https://github.com/malamtime/cli/commit/bea74de6f45b3064eba5d1b163edb1cf7c159d62))


### Bug Fixes

* **docs:** update readme ([8bf7e8f](https://github.com/malamtime/cli/commit/8bf7e8fde4dba2578073701b03b351ef569cf309))


### Miscellaneous Chores

* release 0.0.42 ([5c56570](https://github.com/malamtime/cli/commit/5c56570dbc6e455bbec759130983cce373887016))

## [0.0.41](https://github.com/malamtime/cli/compare/v0.0.40...v0.0.41) (2024-12-10)


### Bug Fixes

* **docs:** add shelltime badge ([98e4529](https://github.com/malamtime/cli/commit/98e45296f07b7ab53cf9a4a7a479542750bfa757))

## [0.0.40](https://github.com/malamtime/cli/compare/v0.0.39...v0.0.40) (2024-12-10)


### Performance Improvements

* **api:** reduce tracking data size for performance improve ([c73c17a](https://github.com/malamtime/cli/commit/c73c17ab24f7f95da56576722f586e11d6fd43c4))

## [0.0.39](https://github.com/malamtime/cli/compare/v0.0.38...v0.0.39) (2024-12-09)


### Bug Fixes

* **handshake:** fix check method of handshake ([fd1af55](https://github.com/malamtime/cli/commit/fd1af55f433b0a93c7cfb5df169d58227ec2639d))

## [0.0.38](https://github.com/malamtime/cli/compare/v0.0.37...v0.0.38) (2024-12-09)


### Features

* **handshake:** add handshake support for smooth auth ([c9d8338](https://github.com/malamtime/cli/commit/c9d833819f0b5d26bdf7f0be30ebc90cb703b3f8))


### Bug Fixes

* **version:** remove legacy version info ([31d0c59](https://github.com/malamtime/cli/commit/31d0c5971c0d2e26a680c1f544080f29e6dfe487))


### Miscellaneous Chores

* release 0.0.38 ([412fdc5](https://github.com/malamtime/cli/commit/412fdc50cc56faa21859ae263c61fdc41634f6cd))

## [0.0.37](https://github.com/malamtime/cli/compare/v0.0.36...v0.0.37) (2024-12-07)


### Bug Fixes

* **api:** add testcase for model/api ([925ced5](https://github.com/malamtime/cli/commit/925ced5ad031d13bae467c7714b686cf48852a0d))
* **http:** add timeout on http send req ([4f7880c](https://github.com/malamtime/cli/commit/4f7880ca927f252a0f95e2084ff72e48197006f6))
* **http:** support multiple endpoint for debug ([1251f65](https://github.com/malamtime/cli/commit/1251f65ad95faf582128ac29cc9a4b2d737dde8e))

## [0.0.36](https://github.com/malamtime/cli/compare/v0.0.35...v0.0.36) (2024-12-04)


### Bug Fixes

* **build:** use default ldflags on build ([82cc3ca](https://github.com/malamtime/cli/commit/82cc3ca861aed8da3ffddf3b8d5d4b74a91137fe))

## [0.0.35](https://github.com/malamtime/cli/compare/v0.0.34...v0.0.35) (2024-12-04)


### Bug Fixes

* **ci:** update metadata in main ([28b2bbf](https://github.com/malamtime/cli/commit/28b2bbf8030701eb62f10d6c22cd191f42c52e8d))

## [0.0.34](https://github.com/malamtime/cli/compare/v0.0.33...v0.0.34) (2024-12-01)


### Bug Fixes

* **track:** fix first load tests on track ([0f30741](https://github.com/malamtime/cli/commit/0f3074188a0d8fc6b6bef4b9d25f78251008393e))

## [0.0.33](https://github.com/malamtime/cli/compare/v0.0.32...v0.0.33) (2024-11-24)


### Bug Fixes

* **track:** allow very first command sync to server ([676ece3](https://github.com/malamtime/cli/commit/676ece3340c166d222a43cda496c287846d27d78))

## [0.0.32](https://github.com/malamtime/cli/compare/v0.0.31...v0.0.32) (2024-11-21)


### Features

* **track:** add data masking for sensitive token ([bb2460a](https://github.com/malamtime/cli/commit/bb2460af3e1bc2c55d86f05d0880138d1a2f9e57))


### Bug Fixes

* **os:** correct os info in linux ([c9f5f98](https://github.com/malamtime/cli/commit/c9f5f98bdc38e5b6b01bd8c0246afd9a4f92f2b9))


### Miscellaneous Chores

* release 0.0.32 ([d298613](https://github.com/malamtime/cli/commit/d2986139e434947746811c39a73e7606f83faf8d))

## [0.0.31](https://github.com/malamtime/cli/compare/v0.0.30...v0.0.31) (2024-11-19)


### Bug Fixes

* **gc:** check original file exists before rename it ([3263d47](https://github.com/malamtime/cli/commit/3263d47df98ac91ae69d7e29df4d5e4c373ede71))

## [0.0.30](https://github.com/malamtime/cli/compare/v0.0.29...v0.0.30) (2024-11-16)


### Bug Fixes

* **api:** fix testcase on msgpack decode ([15e9c92](https://github.com/malamtime/cli/commit/15e9c9270eabe8f54bf76d0773ea97e438fd4854))
* **api:** support msgpack in api ([8656c15](https://github.com/malamtime/cli/commit/8656c155a73b96f34899ebb411a29b7bf4881abe))
* **ci:** use release action tag name instead of github ci one ([75fb9bf](https://github.com/malamtime/cli/commit/75fb9bfcb33496bfe36aba079295120c50796712))

## [0.0.29](https://github.com/malamtime/cli/compare/v0.0.28...v0.0.29) (2024-11-16)


### Bug Fixes

* **brand:** fix config folder combination ([7642c97](https://github.com/malamtime/cli/commit/7642c97e0bdfa1108de8408fb2784928016e1153))

## [0.0.28](https://github.com/malamtime/cli/compare/v0.0.27...v0.0.28) (2024-11-16)


### Bug Fixes

* **brand:** rename to shelltime.xyz ([79099e4](https://github.com/malamtime/cli/commit/79099e4a207ae703f58bf52298122163cf07b71e))
* **brand:** rename to shelltime.xyz ([1336fd9](https://github.com/malamtime/cli/commit/1336fd9856ee11a8cc60f6f25535b8043a5553ac))
* **cli:** add version field ([1077519](https://github.com/malamtime/cli/commit/10775191428a76ac2d2c7ac69675dc724e980c15))

## [0.0.27](https://github.com/malamtime/cli/compare/v0.0.26...v0.0.27) (2024-11-16)


### Bug Fixes

* add os and osVersion to tracking data ([68f5f21](https://github.com/malamtime/cli/commit/68f5f214daf20b8a4ca0708633c6935a3ba2f4e9))

## [0.0.26](https://github.com/malamtime/cli/compare/v0.0.25...v0.0.26) (2024-11-10)


### Bug Fixes

* **ci:** add tag to binary ([8b583ed](https://github.com/malamtime/cli/commit/8b583ed08df7754a81707c3c786d37ead17f58e7))

## [0.0.25](https://github.com/malamtime/cli/compare/v0.0.24...v0.0.25) (2024-10-20)


### Bug Fixes

* **track:** fix cursor writer ([fb0cd4d](https://github.com/malamtime/cli/commit/fb0cd4de5ddc1b750e65056bc7175deacfa79449))

## [0.0.24](https://github.com/malamtime/cli/compare/v0.0.23...v0.0.24) (2024-10-19)


### Bug Fixes

* **logger:** close logger only on program close ([187ec6f](https://github.com/malamtime/cli/commit/187ec6ffc007f7bdca92cdf68b40e3fa8064a9eb))

## [0.0.23](https://github.com/malamtime/cli/compare/v0.0.22...v0.0.23) (2024-10-15)


### Bug Fixes

* **db:** ignore empty line on db ([67cdcd5](https://github.com/malamtime/cli/commit/67cdcd5d4ed6b6d381a29e54e2a26e3fff0697c2))
* **db:** use load-once to avoid buffer based parse ([97af089](https://github.com/malamtime/cli/commit/97af0892d0cf634d54daa9888b7be80b78545560))
* **tests:** skip logger settings in testing ([9c9e8ed](https://github.com/malamtime/cli/commit/9c9e8ed7b6af83c5495cac593d70b918157eecd0))

## [0.0.22](https://github.com/malamtime/cli/compare/v0.0.21...v0.0.22) (2024-10-14)


### Bug Fixes

* **db:** fix line parser ([4f01d88](https://github.com/malamtime/cli/commit/4f01d8843035aafee5076f417a45cd615c2e6d8f))
* **log:** enable log on each command ([1374164](https://github.com/malamtime/cli/commit/1374164d736e4fa1e672bc8a57f8fc92be18a841))

## [0.0.21](https://github.com/malamtime/cli/compare/v0.0.20...v0.0.21) (2024-10-13)


### Bug Fixes

* **db:** handle cursor file data not found issue ([6bc2adb](https://github.com/malamtime/cli/commit/6bc2adb9df2eba9e9a91011581957df0c294d0a6))

## [0.0.20](https://github.com/malamtime/cli/compare/v0.0.19...v0.0.20) (2024-10-13)


### Bug Fixes

* **gc:** fix closest node check ([f93a8fb](https://github.com/malamtime/cli/commit/f93a8fb20c73c580f050ed1d207342733ac1ac1a))
* **gc:** fix gc command remove incorrectly unfinished pre commands issue ([2aa7663](https://github.com/malamtime/cli/commit/2aa7663b7c32f38ce615358cd6c17840e75920b4))

## [0.0.19](https://github.com/malamtime/cli/compare/v0.0.18...v0.0.19) (2024-10-13)


### Bug Fixes

* **api:** not parse api response if it's ok ([5d40712](https://github.com/malamtime/cli/commit/5d407126338dd43960726ca31f71cf6e72ef2809))
* **ci:** disable race in testing ([09eaa83](https://github.com/malamtime/cli/commit/09eaa8375c4535eaff3024903f81501f0dcacab7))
* **docs:** add testing badge to readme ([45e8e28](https://github.com/malamtime/cli/commit/45e8e28c3dfd3a085f924b88613d7699073a8a00))
* **gc:** clean pre, post and cursor in gc command ([b56676a](https://github.com/malamtime/cli/commit/b56676a47d8af279901924e4fa8a2378dd290a14))
* **gc:** fix gc command issue and add tests ([9a8ed39](https://github.com/malamtime/cli/commit/9a8ed392cde80e37947535cf258339d1ebb72c23))
* **track:** fix issue that could be sync data more than once ([c822898](https://github.com/malamtime/cli/commit/c8228988092065b5bccfee323b7703a8717f6946))


### Performance Improvements

* **mod:** remove unused mod ([44d2121](https://github.com/malamtime/cli/commit/44d21217c31bb271103b6d17d7dc38c1673f2c74))
* **tracking:** check the pair of pre and post command and sync to server ([3c3a13b](https://github.com/malamtime/cli/commit/3c3a13b894559f502410180d695e775d19d9c77f))
* **track:** use append file to improve performance ([6e361b4](https://github.com/malamtime/cli/commit/6e361b436c1c9389c48ce17d9e6c6531eb85491a))

## [0.0.18](https://github.com/malamtime/cli/compare/v0.0.17...v0.0.18) (2024-10-11)


### Bug Fixes

* **db:** ignore db close error ([0a2c41f](https://github.com/malamtime/cli/commit/0a2c41f11f9d2e4bb9e552afb8c93d6284121d0b))
* **http:** change auth method for http client ([ef398d9](https://github.com/malamtime/cli/commit/ef398d90c756507a7fde26f2f714dca56c7eb25d))

## [0.0.17](https://github.com/malamtime/cli/compare/v0.0.16...v0.0.17) (2024-10-05)


### Bug Fixes

* **log:** ignore panic and put it in log file silently ([7ca505b](https://github.com/malamtime/cli/commit/7ca505bee629da3383fdbcce827409b0c1d20d9a))

## [0.0.16](https://github.com/malamtime/cli/compare/v0.0.15...v0.0.16) (2024-10-05)


### Bug Fixes

* **ci:** fix missing id ([19d5f13](https://github.com/malamtime/cli/commit/19d5f13ef8dfcab7146edf86504e51e53bbaabc8))

## [0.0.15](https://github.com/malamtime/cli/compare/v0.0.14...v0.0.15) (2024-10-05)


### Bug Fixes

* **ci:** enable zip ([5c3464b](https://github.com/malamtime/cli/commit/5c3464b865be3e31e1b8c5cb67ddb2a6c9a7e2ef))

## [0.0.14](https://github.com/malamtime/cli/compare/v0.0.13...v0.0.14) (2024-10-05)


### Bug Fixes

* **ci:** enable sign and nortary ([3a73076](https://github.com/malamtime/cli/commit/3a73076edcc7e3adb3cf508d4b7e69b9711d638a))

## [0.0.13](https://github.com/malamtime/cli/compare/v0.0.12...v0.0.13) (2024-10-05)


### Bug Fixes

* **ci:** fix release config ([da230cf](https://github.com/malamtime/cli/commit/da230cf7aba05bc5b2a6a23c75bd356b21bb73f2))

## [0.0.12](https://github.com/malamtime/cli/compare/v0.0.11...v0.0.12) (2024-10-05)


### Bug Fixes

* **ci:** lock releaser version ([c6e2a21](https://github.com/malamtime/cli/commit/c6e2a21305cf3e3a4022266b03300fd87425a122))

## [0.0.11](https://github.com/malamtime/cli/compare/v0.0.10...v0.0.11) (2024-10-05)


### Features

* **gc:** add gc command ([533044f](https://github.com/malamtime/cli/commit/533044fb10f6eeb4631d670dbca82ca8ae04dc5d))


### Bug Fixes

* **db:** fix db api and log permission issue ([e37dbc4](https://github.com/malamtime/cli/commit/e37dbc456e0c91febd0049c7ed0361aa2e728491))
* **gc:** fix syntax error and update parameters of gc command ([329566e](https://github.com/malamtime/cli/commit/329566ec096d3b37e65f40ca08c7e5ca23a1b9f0))


### Performance Improvements

* **db:** migrate to NutsDB since sqlite is too hard to compile ([f765e7b](https://github.com/malamtime/cli/commit/f765e7bc6500eec805b9e117913984873f8c0a4c))


### Miscellaneous Chores

* release 0.0.11 ([b96d666](https://github.com/malamtime/cli/commit/b96d6663cee287058475d9caed17fb775072368e))

## [0.0.10](https://github.com/malamtime/cli/compare/v0.0.9...v0.0.10) (2024-10-05)


### Bug Fixes

* **ci:** ignore coverage.txt for release ([39b7994](https://github.com/malamtime/cli/commit/39b7994cdb89d68cf87fa075ad43034cec2bd2a4))

## [0.0.9](https://github.com/malamtime/cli/compare/v0.0.8...v0.0.9) (2024-10-05)


### Bug Fixes

* **ci:** use quill for binary sign and nortary ([f885733](https://github.com/malamtime/cli/commit/f8857334545c24f61472ad08da2da94c1b1df0b0))

## [0.0.8](https://github.com/malamtime/cli/compare/v0.0.7...v0.0.8) (2024-10-05)


### Bug Fixes

* **ci:** disable windows build ([e15c7ab](https://github.com/malamtime/cli/commit/e15c7ab33d26c76306a4d85f09f1a948534c0e59))

## [0.0.7](https://github.com/malamtime/cli/compare/v0.0.6...v0.0.7) (2024-10-05)


### Bug Fixes

* **ci:** enable cgo for sqlite ([d3d662e](https://github.com/malamtime/cli/commit/d3d662e26e8b06006ae919ec1979807afa117620))

## [0.0.6](https://github.com/malamtime/cli/compare/v0.0.5...v0.0.6) (2024-10-05)


### Bug Fixes

* **ci:** enable cgo for sqlite ([374e5c3](https://github.com/malamtime/cli/commit/374e5c3da4965b181d51dd1d1408ce2dae6db5ab))

## [0.0.5](https://github.com/malamtime/cli/compare/v0.0.4...v0.0.5) (2024-10-05)


### Performance Improvements

* **track:** save to fs first for performance purpose and sync it later ([57c5606](https://github.com/malamtime/cli/commit/57c56066f92ca22b289ddf233657107b72afdbc0))

## [0.0.4](https://github.com/malamtime/cli/compare/v0.0.3...v0.0.4) (2024-10-04)


### Bug Fixes

* **logger:** add logger for app ([0a9821f](https://github.com/malamtime/cli/commit/0a9821ff2c876f6a0f621df3c15ec28197009ed3))
* **track:** fix track method and logger ([3e52212](https://github.com/malamtime/cli/commit/3e5221280586649bb674ff190743a8412798dbff))

## [0.0.3](https://github.com/malamtime/cli/compare/v0.0.2...v0.0.3) (2024-10-04)


### Bug Fixes

* **track:** add tip on track command ([000a367](https://github.com/malamtime/cli/commit/000a367e73fcfdff566f3eeb2a7e9e5d5242ad18))

## [0.0.2](https://github.com/malamtime/cli/compare/v0.0.1...v0.0.2) (2024-10-04)


### Bug Fixes

* **ci:** fix release command ([1ea4e73](https://github.com/malamtime/cli/commit/1ea4e730c5ab3abe0220be1a208fd295da6d3c2b))

## 0.0.1 (2024-10-04)


### Features

* add track command ([f85660a](https://github.com/malamtime/cli/commit/f85660a63f83c69229fe1d4a4b534a1c76f49b58))
* **app:** add basic command and ci ([976973f](https://github.com/malamtime/cli/commit/976973fde38bd054cbdcff9de26b80b73c855892))


### Bug Fixes

* **ci:** fix ci branch name ([5b08dd8](https://github.com/malamtime/cli/commit/5b08dd85d0d818cd1d7d5686ffeb03303d4b00ae))
* **track:** add more params for track ([24de070](https://github.com/malamtime/cli/commit/24de070375e2acec0aaec479387d50baa42b2561))


### Miscellaneous Chores

* release 0.0.1 ([5517712](https://github.com/malamtime/cli/commit/5517712672634a2d7fb5e1438028b1f3a58beb02))
