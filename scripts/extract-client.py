#!/usr/bin/env python3
"""Package one client for its own server.

`docs/isolating-a-client.md` is the procedure; this script does the bookkeeping
the procedure otherwise asks a human to do by hand: enumerate the definitions to
carry, point at the env file whose variable names must be recreated, list the
trigger ids to disable on the old host, and name the external URLs to repoint.

READ-ONLY. It prints a plan (optionally writes it to a file); it never registers,
pushes, disables a trigger or touches another host. Deploying is the operator's
move — repo rule: propose the exact command, wait.

It deliberately does not parse `env/<client>.env.example`. Listing variable
names means reading a file that sits next to real values, and the plan does not
need the contents: it points at the file and the operator reads it.

Usage:
    ./scripts/extract-client.py <client>              # print the plan
    ./scripts/extract-client.py <client> --out DIR    # + write DIR/extract-<client>.md
    TURVEZ_OPS_DIR=../turvez-ops ./scripts/extract-client.py acme
"""

import argparse
import json
import os
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
TEMPLATE = HERE.parent


def manifest_fields(path):
    """Read the flat `key: value` subset of YAML that client.yaml uses.

    Deliberately not a YAML parser: the manifest is flat by design
    (docs/client-spine.md), so a handful of keys do not justify a dependency.
    """
    out = {}
    if not path.is_file():
        return out
    for raw in path.read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        key, sep, value = line.partition(":")
        if not sep:
            continue
        out[key.strip()] = value.split("#", 1)[0].strip().strip("\"'")
    return out


def naming_separator():
    try:
        return json.loads((TEMPLATE / "naming.json").read_text())["client_workflow_separator"]
    except Exception:
        return "."


def pinned_dagnats():
    gomod = TEMPLATE / "go.mod"
    if gomod.is_file():
        for line in gomod.read_text().splitlines():
            parts = line.split()
            if len(parts) >= 2 and parts[0] == "github.com/danmestas/dagnats":
                return parts[1]
    return "latest"


def definitions_for(cdir, engine):
    if engine == "dagnats":
        found = sorted((cdir / "automation" / "dagnats").glob("*.json"))
        return found + sorted(cdir.glob("*.json"))  # legacy flat layout
    if engine == "windmill":
        wm = cdir / "automation" / "windmill"
        return sorted(p for p in wm.glob("*") if p.is_dir()) if wm.is_dir() else []
    if engine == "dagu":
        return sorted((cdir / "automation" / "dagu").glob("*.yaml"))
    return []


def workflow_name(path, engine, client):
    if engine == "windmill":
        return "f/%s/%s" % (client, path.name)
    if engine == "dagu":
        return path.stem
    try:
        return json.loads(path.read_text()).get("name", "")
    except Exception:
        return ""


def trigger_ids(path):
    try:
        wf = json.loads(path.read_text())
    except Exception:
        return []
    out = []
    for trig in wf.get("triggers") or []:
        ident = trig.get("id") or trig.get("name")
        if ident:
            out.append(ident)
    return out


def new_server_commands(client, engine, defs, root):
    lines = ["```bash"]
    if engine == "dagnats":
        lines += [
            "# same pinned engine version as the old host",
            "go install github.com/danmestas/dagnats/cmd/dagnats@%s" % pinned_dagnats(),
            "dagnats serve &",
            "# worker loads only this client",
            "export CLIENT_ID=%s" % client,
            "go run ./cmd/worker &",
            "# definitions, prefix-checked by the script",
            "./scripts/register-workflows.sh %s" % client,
        ]
    else:
        lines += [
            "# NEVER wmill sync push - it deletes remote items missing locally.",
            "# Push one flow at a time, after a local preview.",
        ]
        for path in defs:
            # Paths are relative to the ops root so the printed commands work from
            # a checkout at any location; an absolute /private/tmp/... path would
            # only be correct on the machine that generated the plan.
            try:
                rel = path.relative_to(root)
            except ValueError:
                rel = path
            lines.append('wmill flow preview "%s" && wmill flow push "%s" "f/%s/%s"'
                         % (rel, rel, client, path.name))
        lines += [
            "# secrets: recreate as workspace variables/resources, then confirm the",
            "# flow reads no env file at runtime (the age file is bootstrap only).",
        ]
    lines.append("```")
    return lines


def build_plan(client, root):
    cdir = root / "clients" / client
    if not cdir.is_dir():
        raise SystemExit("no client %r under %s" % (client, root / "clients"))

    manifest = manifest_fields(cdir / "client.yaml")
    engine = manifest.get("engine") or "unknown"
    runtime = manifest.get("runtime") or "shared"
    host = manifest.get("host", "")
    sep = naming_separator()

    defs = definitions_for(cdir, engine)
    names = [n for n in (workflow_name(p, engine, client) for p in defs) if n]
    env_example = root / "env" / ("%s.env.example" % client)
    age = root / "secrets" / client / "env.age"
    trigs = []
    for path in defs:
        if path.suffix == ".json":
            trigs.extend(trigger_ids(path))

    L = []
    add = L.append
    add("# Extract %s to its own server" % client)
    add("")
    add("- engine: **%s**" % engine)
    add("- runtime declared: **%s**%s" % (runtime, (" (target: %s)" % host) if host else ""))
    add("- definitions: %d" % len(defs))
    add("")

    add("## 1. Definitions to carry (from git, never by hand)")
    add("")
    if not defs:
        add("_none found_ - does `%s/automation/%s/` exist?" % (cdir.name, engine))
    else:
        for path in defs:
            add("- `%s`" % path.relative_to(root))
    add("")

    add("## 2. Secrets")
    add("")
    if env_example.is_file():
        add("Recreate every variable declared in `%s` on the new host."
            % env_example.relative_to(root))
        add("Read the file directly; this plan deliberately does not echo it.")
    else:
        add("_no `env/%s.env.example` - create it before cutover_" % client)
    if age.is_file():
        add("")
        add("Ciphertext: `%s` (decrypt out-of-band with the age identity)."
            % age.relative_to(root))
    add("")

    add("## 3. Commands - NEW server")
    add("")
    L.extend(new_server_commands(client, engine, defs, root))
    if names:
        add("")
        add("Expect ONLY these workflow names:")
        add("")
        for n in names:
            add("- `%s`" % n)
    add("")

    add("## 4. Commands - OLD server (disable only this client)")
    add("")
    if trigs:
        add("```bash")
        for t in trigs:
            add("# disable: %s" % t)
        add("```")
    elif engine == "dagnats":
        add("```bash")
        add("dagnats trigger list   # disable ids starting with %s%s" % (client, sep))
        add("```")
    else:
        add("Disable only this client's workspace schedules; leave other workspaces alone.")
    add("")

    add("## 5. Repoint external systems")
    add("")
    add("SaaS senders must point at the new host. Grep the definitions above for")
    add("webhook paths containing `%s`, then re-run the smoke test against the NEW" % client)
    add("host before disabling anything on the old one.")
    add("")

    add("## 6. Rollback")
    add("")
    add("Re-enable on the old server, repoint the external URLs back, disable on the")
    add("new one. Definitions are in git, so nothing is lost in either direction.")
    add("Run history never moves.")
    add("")

    add("## 7. Finish the manifest")
    add("")
    add("Once the new host is live, record it:")
    add("")
    add("```yaml")
    add("runtime: dedicated")
    add("target:")
    add("  host: <new-host>")
    add("```")
    add("")
    add("Then `./scripts/verify-isolation.sh %s` must print coherent." % client)
    return L


def main():
    ap = argparse.ArgumentParser(description="Package a client for its own server.")
    ap.add_argument("client")
    ap.add_argument("--out", help="write the plan to this directory")
    args = ap.parse_args()

    if not re.fullmatch(r"[a-z0-9][a-z0-9-]*", args.client):
        raise SystemExit("bad client id: %s (lowercase, digits, dashes)" % args.client)

    root = Path(os.environ.get("TURVEZ_OPS_DIR") or TEMPLATE).resolve()
    text = "\n".join(build_plan(args.client, root)) + "\n"

    if args.out:
        out = Path(args.out)
        out.mkdir(parents=True, exist_ok=True)
        dest = out / ("extract-%s.md" % args.client)
        dest.write_text(text)
        sys.stderr.write("wrote %s\n" % dest)
    else:
        sys.stdout.write(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
