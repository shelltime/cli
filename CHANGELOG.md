# Changelog

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
