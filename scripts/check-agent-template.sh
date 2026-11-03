#!/usr/bin/env bash
# check-agent-template.sh — compile the Genkit scaffold for real.
#
# scripts/test-engines.sh proves the generated main.go parses and carries the
# right contracts. It cannot prove it BUILDS: Genkit is deliberately not in
# go.mod, so the scaffold shipped against an API it had never been compiled
# against — and did not compile (`ai.Model` vs goai's `provider.LanguageModel`,
# and `genkit.LookupFlow`, which v1.14 does not have).
#
# This gate closes that hole the honest way: it runs new-agent-service.sh into
# a throwaway ops dir, then builds the file the operator actually gets, in a
# temp module. It is the ONLY place that downloads Genkit — never go.mod,
# always a temp dir that is deleted, so every other build stays lean.
#
# The version is pinned here, not `@latest`: a build must be reproducible, and
# a deliberate bump is what proves the scaffold still matches the API.
#
#   ./scripts/check-agent-template.sh
#   GENKIT_VERSION=v1.15.0 ./scripts/check-agent-template.sh   # bump check
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
VERSION="${GENKIT_VERSION:-v1.14.0}"

command -v go >/dev/null 2>&1 || { echo "check-agent-template: go not found" >&2; exit 1; }
[ -d "$ROOT/clients/example-client" ] || { echo "check-agent-template: missing clients/example-client fixture" >&2; exit 1; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/clients"
cp -R "$ROOT/clients/example-client" "$tmp/clients/"
cp "$ROOT/engines.json" "$tmp/"

# The real path: plan output is not enough, the written file is what builds.
TURVEZ_OPS_DIR="$tmp" "$HERE/new-agent-service.sh" example-client --apply >/dev/null

cd "$tmp"
go mod init agentsvc-check >/dev/null 2>&1
if ! go get "github.com/firebase/genkit/go@$VERSION" >/dev/null 2>&1; then
  echo "check-agent-template: cannot fetch genkit $VERSION (network?)" >&2
  exit 1
fi
go mod tidy >/dev/null 2>&1

if [ -n "$(gofmt -l ./cmd)" ]; then
  echo "check-agent-template: the scaffold is not gofmt-clean:" >&2
  gofmt -l ./cmd >&2
  exit 1
fi
if ! go build ./... 2>"$tmp/build.err"; then
  echo "check-agent-template: the scaffold does not compile against genkit $VERSION:" >&2
  cat "$tmp/build.err" >&2
  exit 1
fi
if ! go vet ./... 2>"$tmp/vet.err"; then
  echo "check-agent-template: go vet failed:" >&2
  cat "$tmp/vet.err" >&2
  exit 1
fi

echo "ok: agent scaffold compiles and vets (genkit $VERSION)"
