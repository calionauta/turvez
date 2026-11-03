## What

<!-- One paragraph: what changes, and which engine/client it touches. -->

## Why

<!-- The problem or the decision this implements. Link the doc if there is one. -->

## Verification

<!-- What you actually ran, not what you intend to run. -->

- [ ] `go test ./...`
- [ ] `make validate` (every engine with source in this checkout)
- [ ] `./scripts/test-decisions.sh` (if a decision-tree branch changed)
- [ ] Secrets: names only in JSON/env — no values committed

## Checklist

- [ ] One engine per client (never two engines side by side)
- [ ] Conventional Commit subject (`feat:`, `fix:`, `docs:`, `chore:`)
- [ ] No `1.x` version bump (the project stays `0.x` until a human asks)
- [ ] Docs updated when a command, path, or gate changed
