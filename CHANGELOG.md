# Changelog

## [0.1.1](https://github.com/calionauta/turvez/compare/v0.1.0...v0.1.1) (2026-10-24)


### Documentation

* pin ops repos to release tags ([2ccde75](https://github.com/calionauta/turvez/commit/2ccde75e15368985ccea32ed9fae06e8cf46989d))

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
