# Changelog

## [0.1.3](https://github.com/calionauta/turvez/compare/v0.1.2...v0.1.3) (2026-10-31)


### Features

* add the client engine manifest and the Windmill gate ([9f3e0e4](https://github.com/calionauta/turvez/commit/9f3e0e4be8d1912bc7bea0831c01e6963f2a584c))
* add the Windmill lint gate ([c838581](https://github.com/calionauta/turvez/commit/c838581d54025b0be8c4d18992a59fad8fbde71c))
* **eval:** add per-client eval scaffold, case selector and stack doctor ([83daae9](https://github.com/calionauta/turvez/commit/83daae9692f8aa084340cd86e2a27a0d87f2fa7b))
* **eval:** generic eval config, labeling and one-command loop ([d5dc91c](https://github.com/calionauta/turvez/commit/d5dc91c2031a60f96521e8eb6cddb675730fd76f))
* **eval:** import unlabeled logs and configure the judge ([68ef7e1](https://github.com/calionauta/turvez/commit/68ef7e17a196361465f7ac45a18d668b79c4c9ab))
* **eval:** normalize Windmill jobs for the case selector ([bbee385](https://github.com/calionauta/turvez/commit/bbee3850972fd0dffad320019e4252c9f6e126f8))
* **llm:** route llm.complete through GoAI with explicit retry and error contracts ([eba7eaf](https://github.com/calionauta/turvez/commit/eba7eaf54af8d921562348d3b9c4c9ddf952eea6))
* support the clients/ spine layout alongside workflows/ ([2db327e](https://github.com/calionauta/turvez/commit/2db327eac666541f7bf62184be018c74a6d2cd39))


### Bug Fixes

* **compose:** start the server, parameterize host ports, enable console auth ([3f3ba1a](https://github.com/calionauta/turvez/commit/3f3ba1aa15e4390dbad140d16127380b8d3fd4f8))
* make the two-engine claim actually true ([61d737c](https://github.com/calionauta/turvez/commit/61d737c5befbeabf9b82cad42f033b3cc31110cd))


### Code Refactoring

* **naming:** single-source workflow names via naming.json ([d00eba2](https://github.com/calionauta/turvez/commit/d00eba2f4f8f72861e522fc65161c0e8d0912e30))
* one client spine, dagnats declared legacy (phases 3-4) ([9959fa5](https://github.com/calionauta/turvez/commit/9959fa52c73a0fac5a83093191504b8f3694a5da))


### Documentation

* add first-run, deployment profiles and observability rules ([fba78db](https://github.com/calionauta/turvez/commit/fba78db9d8c4d327caf603cdab94236e7b473fbf))
* add the engine-agnostic client spine ([4de43e9](https://github.com/calionauta/turvez/commit/4de43e9c82e22073558bf1a9858c4efc815da003))
* add the operator sequence to the workflow diagrams ([c4fd075](https://github.com/calionauta/turvez/commit/c4fd0756d370ca230d29845a41b1504d84412395))
* add the platform choice matrix (dagnats, Windmill, n8n, Dagu, Nango) ([274cb0a](https://github.com/calionauta/turvez/commit/274cb0a2e452dad294f49c8d84aba357bd6ff68e))
* add Windmill topology and the auth-layer gate ([ec2578e](https://github.com/calionauta/turvez/commit/ec2578e8df4d6e914eaa35ea79ccfbfbdcc0605e))
* add workflow diagrams and the format map ([4dce700](https://github.com/calionauta/turvez/commit/4dce700e8ddd7c85b2b0de888cfe5a4958a77886))
* run filters are exact names, not prefixes ([1bb813d](https://github.com/calionauta/turvez/commit/1bb813d8c4ea6e5f6c1950a43f247579c2fefb35))
* **skill:** document trigger creation and behavior ([5f2a2fe](https://github.com/calionauta/turvez/commit/5f2a2fe2ef5e8cad2fdb056b03868711a3ec0da3))
* **skill:** link the auth-layer reference ([1d42a0e](https://github.com/calionauta/turvez/commit/1d42a0e57242d32a7344153f4685b0fde3b88656))
* **skill:** map the tooling and eval options, translate examples ([8c93fd8](https://github.com/calionauta/turvez/commit/8c93fd8be7138d2e0c9b3373b8b6f63ef944a190))

## [0.1.2](https://github.com/calionauta/turvez/compare/v0.1.1...v0.1.2) (2026-10-24)


### Features

* guard the pre-1.0 policy and harden CI ([5d7d25e](https://github.com/calionauta/turvez/commit/5d7d25eaf1221418e5fe046d47390b270b897929))

## [0.1.1](https://github.com/calionauta/turvez/compare/v0.1.0...v0.1.1) (2026-10-24)


### Features

* enforce Conventional Commits locally and in CI ([3d731db](https://github.com/calionauta/turvez/commit/3d731dbdcd2353d0da60b36e4e05749e56a1324c))

## 0.1.0 (2026-10-24)

### Features

* initial public template: durable dagnats runtime, twelve generic handlers, per-client recipes under `workflows/<client>/`, and native approval/sleep steps
* private ops-repo split with `TURVEZ_PIN`, age-encrypted per-client secrets, and contract tests against a pinned template ref
* `turvez` skill: direct → confirm → build, with a where-to-run decision tree
* CI (test, lint, engine validation) and release-please automation

### Documentation

* refresh the Dagu vs dagnats boundary against Dagu v2.22.2, add the agent-loop gate, and document at-least-once semantics with the three idempotency layers
* ops repos now pin a release tag (`TURVEZ_REF=vX.Y.Z`); SHA pins remain for unreleased commits under test

### Bug Fixes

* resolve tag pins in `sync-template.sh` and scaffold ops repos from an exact tag
* apply gofmt import ordering to handler files
