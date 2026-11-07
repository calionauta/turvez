# Changelog

## [0.1.11](https://github.com/calionauta/turvez/compare/v0.1.10...v0.1.11) (2026-11-07)


### Documentation

* surface PageIndex in the README retrieval row ([7c6c81d](https://github.com/calionauta/turvez/commit/7c6c81dc9543fbb02abaa6655909da0df833f708))

## [0.1.10](https://github.com/calionauta/turvez/compare/v0.1.9...v0.1.10) (2026-11-07)


### Features

* expose pageindex as an eval-gated retrieval alternative ([ebcb769](https://github.com/calionauta/turvez/commit/ebcb7696edc24c1e92e95270bfd029a18982c5a8))


### Documentation

* break the access plane paragraph into bullets ([b9b4e49](https://github.com/calionauta/turvez/commit/b9b4e4936edc5e507b48b800ffe105c2ead7ef3b))
* make the ask-with-question instruction explicit in the skill ([f279f85](https://github.com/calionauta/turvez/commit/f279f859650ad48cd275259edcbaec2a8ae091a2))
* rewrite the README opening as spine / engine table / layers list ([13eb40f](https://github.com/calionauta/turvez/commit/13eb40fd32ca067c19d3b223bd58aac5215a711d))

## [0.1.9](https://github.com/calionauta/turvez/compare/v0.1.8...v0.1.9) (2026-11-07)


### Features

* add the minimal P2 nango adapter ([4082348](https://github.com/calionauta/turvez/commit/408234879854ecbaaf74853a1717557874d17470))
* ask cloud versus self-hosted per layer at onboarding ([#24](https://github.com/calionauta/turvez/issues/24)) ([1126715](https://github.com/calionauta/turvez/commit/1126715e53d3c9bb3f8652378933d0ff840ad274))
* implement p2 grants manager, windmill version note, nango key docs ([bc66e0d](https://github.com/calionauta/turvez/commit/bc66e0d1d6039f83e8bbe698f3f54c1be0afdea1))
* P2-B access plane CLI ([#25](https://github.com/calionauta/turvez/issues/25)) ([95edf83](https://github.com/calionauta/turvez/commit/95edf83c1f4e9a73ad6eee4362b215a6de1f6641))
* the access plane, and a credential broker per client ([#21](https://github.com/calionauta/turvez/issues/21)) ([61e3786](https://github.com/calionauta/turvez/commit/61e3786aea57f6625e79a2f82484d4a1a07bf9ee))
* update only the nango secret key in connect remotes ([3c4868a](https://github.com/calionauta/turvez/commit/3c4868a9dfa20157e0cb136c310e00936e580e16))


### Bug Fixes

* accept bare hosts and use the correct Windmill Cloud URL ([ff55a70](https://github.com/calionauta/turvez/commit/ff55a705b7a8a945eb8f4a9faeffdf7ee3a6abbf))
* close the client onboarding loop for cloud remotes and nango level 2 ([b99438e](https://github.com/calionauta/turvez/commit/b99438e9c15aec4e74256065f7d1f7e8ebb1a87b))
* reject account-level nango keys in connect remotes ([3da3c9a](https://github.com/calionauta/turvez/commit/3da3c9a67d2e7f2e2d16fc867f75ace13288cf95))


### Documentation

* clarify the per-client broker default and shared pilot ([b36072e](https://github.com/calionauta/turvez/commit/b36072eaf871e940d0b91a754fee209e1357ec42))
* close the onboarding loop for cloud remotes and nango level 2 ([78a1b08](https://github.com/calionauta/turvez/commit/78a1b08856b9fd985f3b965afc6a2ad32e8e935f))
* surface the P2-B CLI in skill, llms.txt and README ([2dceb41](https://github.com/calionauta/turvez/commit/2dceb41a649c938152b283cd7e20c09483d8c2b3))

## [0.1.8](https://github.com/calionauta/turvez/compare/v0.1.7...v0.1.8) (2026-11-03)


### Bug Fixes

* **deps:** bump github.com/zendev-sh/goai ([#15](https://github.com/calionauta/turvez/issues/15)) ([dbeab9e](https://github.com/calionauta/turvez/commit/dbeab9edb0ff931dbe05b3d8ddfdaa2f7a9d6c44))

## [0.1.7](https://github.com/calionauta/turvez/compare/v0.1.6...v0.1.7) (2026-11-03)


### Features

* **eval:** grade the decision tree against operator scenarios ([a7056f4](https://github.com/calionauta/turvez/commit/a7056f47cadad698bf3510ee740a0adc9de6cc93))

## [0.1.6](https://github.com/calionauta/turvez/compare/v0.1.5...v0.1.6) (2026-11-03)


### Documentation

* make the decision tree the single description of the stack ([#14](https://github.com/calionauta/turvez/issues/14)) ([4cd224a](https://github.com/calionauta/turvez/commit/4cd224ace635d1f92a832c21e030365d7d13cd5c))

## [0.1.5](https://github.com/calionauta/turvez/compare/v0.1.4...v0.1.5) (2026-11-03)


### Features

* scaffold the Genkit capability, and make the decision tree ask ([#9](https://github.com/calionauta/turvez/issues/9)) ([b4745c0](https://github.com/calionauta/turvez/commit/b4745c015561a5ae94e2ad881fb937939d3bbd44))


### Bug Fixes

* one Genkit pin for scaffold and gate, and a rendered decision tree ([#13](https://github.com/calionauta/turvez/issues/13)) ([5319d24](https://github.com/calionauta/turvez/commit/5319d24e5e561fe3866240adc831f12c3cd5571d))

## [0.1.4](https://github.com/calionauta/turvez/compare/v0.1.3...v0.1.4) (2026-11-01)


### Features

* cut the delivery cost 38% with a fast path, and make env examples committable ([74cb1a9](https://github.com/calionauta/turvez/commit/74cb1a945d0f6d14ea612e876f349fcaad32abb2))


### Bug Fixes

* both migration scripts produced output that could not be used ([50deaae](https://github.com/calionauta/turvez/commit/50deaaea723c33d251dfa9d839588c448a013c23))
* dagu could never pass validation, and the default engine scaffolded empty ([4bcc2f2](https://github.com/calionauta/turvez/commit/4bcc2f2b73815c63fafc6197f7b539453dbe0399))


### Documentation

* map the next bottleneck — a local gate that catches what wmill lint passes ([fda97e9](https://github.com/calionauta/turvez/commit/fda97e9d8a4b6f46ab69fdfdbb470d1fcbb92303))

## [0.1.3](https://github.com/calionauta/turvez/compare/v0.1.2...v0.1.3) (2026-11-01)


### Features

* add the client engine manifest and the Windmill gate ([9f3e0e4](https://github.com/calionauta/turvez/commit/9f3e0e4be8d1912bc7bea0831c01e6963f2a584c))
* add the Windmill lint gate ([c838581](https://github.com/calionauta/turvez/commit/c838581d54025b0be8c4d18992a59fad8fbde71c))
* declare Windmill the default engine, and make isolation verifiable ([472d4f7](https://github.com/calionauta/turvez/commit/472d4f7f98a7cc39ad02e5341ddf2269e8244ed7))
* **eval:** add per-client eval scaffold, case selector and stack doctor ([83daae9](https://github.com/calionauta/turvez/commit/83daae9692f8aa084340cd86e2a27a0d87f2fa7b))
* **eval:** generic eval config, labeling and one-command loop ([d5dc91c](https://github.com/calionauta/turvez/commit/d5dc91c2031a60f96521e8eb6cddb675730fd76f))
* **eval:** import unlabeled logs and configure the judge ([68ef7e1](https://github.com/calionauta/turvez/commit/68ef7e17a196361465f7ac45a18d668b79c4c9ab))
* **eval:** normalize Windmill jobs for the case selector ([bbee385](https://github.com/calionauta/turvez/commit/bbee3850972fd0dffad320019e4252c9f6e126f8))
* **llm:** route llm.complete through GoAI with explicit retry and error contracts ([eba7eaf](https://github.com/calionauta/turvez/commit/eba7eaf54af8d921562348d3b9c4c9ddf952eea6))
* support the clients/ spine layout alongside workflows/ ([2db327e](https://github.com/calionauta/turvez/commit/2db327eac666541f7bf62184be018c74a6d2cd39))


### Bug Fixes

* a trigger without `enabled` never fires, and approval is not a dagnats-only gate ([76d29f4](https://github.com/calionauta/turvez/commit/76d29f4c2fbdf5a89491ef5bdd9ecdd40a672861))
* **compose:** start the server, parameterize host ports, enable console auth ([3f3ba1a](https://github.com/calionauta/turvez/commit/3f3ba1aa15e4390dbad140d16127380b8d3fd4f8))
* engine is a per-client decision, and document the three questions ([1b0bbb6](https://github.com/calionauta/turvez/commit/1b0bbb64f45e3292662273680f26b8f48e3511b0))
* let a Windmill-only checkout pass the recipe contract ([4aab2f7](https://github.com/calionauta/turvez/commit/4aab2f766bef20c2fdeae55f29d270f3d01666a1))
* make the two-engine claim actually true ([61d737c](https://github.com/calionauta/turvez/commit/61d737c5befbeabf9b82cad42f033b3cc31110cd))
* the windmill lint gate validated nothing in an ops repo, and default was ambiguous ([4028b47](https://github.com/calionauta/turvez/commit/4028b477f58c3177697830ed765104782db3b526))


### Code Refactoring

* **naming:** single-source workflow names via naming.json ([d00eba2](https://github.com/calionauta/turvez/commit/d00eba2f4f8f72861e522fc65161c0e8d0912e30))


### Documentation

* add first-run, deployment profiles and observability rules ([fba78db](https://github.com/calionauta/turvez/commit/fba78db9d8c4d327caf603cdab94236e7b473fbf))
* add the engine-agnostic client spine ([4de43e9](https://github.com/calionauta/turvez/commit/4de43e9c82e22073558bf1a9858c4efc815da003))
* add the operator sequence to the workflow diagrams ([c4fd075](https://github.com/calionauta/turvez/commit/c4fd0756d370ca230d29845a41b1504d84412395))
* add the platform choice matrix (dagnats, Windmill, n8n, Dagu, Nango) ([274cb0a](https://github.com/calionauta/turvez/commit/274cb0a2e452dad294f49c8d84aba357bd6ff68e))
* add Windmill topology and the auth-layer gate ([ec2578e](https://github.com/calionauta/turvez/commit/ec2578e8df4d6e914eaa35ea79ccfbfbdcc0605e))
* add workflow diagrams and the format map ([4dce700](https://github.com/calionauta/turvez/commit/4dce700e8ddd7c85b2b0de888cfe5a4958a77886))
* describe the eval adapter that exists, not a --source flag ([68560c3](https://github.com/calionauta/turvez/commit/68560c3e640514950b59acd5231029da3ad76f63))
* run filters are exact names, not prefixes ([1bb813d](https://github.com/calionauta/turvez/commit/1bb813d8c4ea6e5f6c1950a43f247579c2fefb35))
* **skill:** document trigger creation and behavior ([5f2a2fe](https://github.com/calionauta/turvez/commit/5f2a2fe2ef5e8cad2fdb056b03868711a3ec0da3))
* **skill:** link the auth-layer reference ([1d42a0e](https://github.com/calionauta/turvez/commit/1d42a0e57242d32a7344153f4685b0fde3b88656))
* **skill:** map the tooling and eval options, translate examples ([8c93fd8](https://github.com/calionauta/turvez/commit/8c93fd8be7138d2e0c9b3373b8b6f63ef944a190))
* teach the skill to operate the scripts, not just know the policy ([dc656f3](https://github.com/calionauta/turvez/commit/dc656f31e23addcc803f3862db63cf2ffb5515e4))

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
