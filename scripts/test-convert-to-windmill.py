#!/usr/bin/env python3
"""Test the converter's output is valid YAML and lint-clean.

The converter was written, demonstrated once, and its output was never fed back
into the schema it targets. Every generated flow was rejected by `wmill lint`:
`input_transforms` was missing (a required property even for a no-input step),
the graph comments sat inside the `value:` mapping, and the `summary` scalar had
text after a closing quote. Three bugs, all invisible to a demo that only checked
"files were created".

This test pins the two things that were missing:
  1. the YAML parses (no schema server needed, runs in CI)
  2. every step carries the properties the flow schema requires

`wmill lint` needs credentials, so CI checks the shape it would check. Pass
--lint to additionally run the real gate when a workspace is configured.

Usage:
    python3 scripts/test-convert-to-windmill.py
    python3 scripts/test-convert-to-windmill.py --lint
"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

try:
    import yaml
except ImportError:
    print("SKIP: pyyaml not installed (pip install pyyaml)", file=sys.stderr)
    sys.exit(0)

# Properties the flow schema requires on every rawscript module.
REQUIRED = ("type", "input_transforms", "language", "content")


def convert(tmp):
    """Run the converter against the shipped example client."""
    src = ROOT / "clients" / "example-client"
    if not src.is_dir():
        print("FAIL: clients/example-client missing — no fixture to convert",
              file=sys.stderr)
        return None
    env = dict(os.environ, TURVEZ_OPS_DIR=str(tmp))
    (tmp / "clients").mkdir(parents=True, exist_ok=True)
    shutil.copytree(src, tmp / "clients" / "example-client", dirs_exist_ok=True)
    out = subprocess.run(
        [sys.executable, str(ROOT / "scripts" / "convert-to-windmill.py"),
         "example-client"],
        env=env, capture_output=True, text=True)
    if out.returncode != 0:
        print("FAIL: converter exited %d\n%s" % (out.returncode, out.stderr),
              file=sys.stderr)
        return None
    return tmp / "clients" / "example-client" / "converted-windmill"


def check(converted, run_lint):
    failures = []
    flows = sorted(p for p in converted.glob("*/flow.yaml"))
    if not flows:
        failures.append("converter produced no flows at all")
        return failures

    for flow in flows:
        name = flow.parent.name
        try:
            doc = yaml.safe_load(flow.read_text())
        except Exception as exc:
            failures.append("%s: invalid YAML — %s" % (name, exc))
            continue

        modules = ((doc or {}).get("value") or {}).get("modules") or []
        if not modules:
            failures.append("%s: no modules" % name)
            continue
        for i, mod in enumerate(modules):
            val = mod.get("value") or {}
            for prop in REQUIRED:
                if prop not in val:
                    failures.append(
                        "%s: module %d (%s) missing required property %r"
                        % (name, i, mod.get("id", "?"), prop))
            # `!inline <file>` must point at a file that exists, or the push
            # uploads a flow referencing nothing.
            content = val.get("content", "")
            if isinstance(content, str) and content.startswith("!inline "):
                target = flow.parent / content[len("!inline "):].strip()
                if not target.is_file():
                    failures.append("%s: inline target %s does not exist"
                                    % (name, target.name))

        if run_lint:
            res = subprocess.run(
                ["wmill", "lint", str(flow.parent)],
                capture_output=True, text=True)
            if "Lint failed" in res.stdout or "Invalid files" in res.stdout:
                failures.append("%s: wmill lint failed — %s"
                                % (name, res.stdout.strip().splitlines()[-1]))
    return failures


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--lint", action="store_true",
                    help="also run `wmill lint` (needs a configured workspace)")
    args = ap.parse_args()

    with tempfile.TemporaryDirectory() as td:
        converted = convert(Path(td))
        if converted is None:
            return 1
        failures = check(converted, args.lint)

    if failures:
        print("FAIL: converter output is not schema-valid:", file=sys.stderr)
        for f in failures:
            print("  - %s" % f, file=sys.stderr)
        return 1
    print("ok: converter output is valid YAML with every required property")
    return 0


if __name__ == "__main__":
    sys.exit(main())
