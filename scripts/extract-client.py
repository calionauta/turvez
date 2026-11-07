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


def access_block(manifest_path):
    """Read the `access:` block of client.yaml.

    manifest_fields() is deliberately flat, and the access plane is the first
    nested block, so a flat read would let any future `enabled:` key elsewhere
    answer for `access.enabled`. This walks the indentation instead: keys at
    indent > 0 under `access:` belong to the block, and the block ends at the
    next top-level key (docs/access-plane.md).
    """
    out = {}
    if not manifest_path.is_file():
        return out
    inside = False
    for raw in manifest_path.read_text().splitlines():
        line = raw.rstrip()
        if not line.strip() or line.strip().startswith("#"):
            continue
        indented = line[0] in " \t"
        if not indented:
            inside = line.split(":", 1)[0].strip() == "access"
            continue
        if not inside:
            continue
        key, sep, value = line.strip().partition(":")
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
        if not wm.is_dir():
            return []
        # A flow directory is one that has a flow.yaml. Anything else in there
        # (.ruff_cache, __pycache__, a stray dir) is tooling noise — treating it
        # as a flow produced `wmill flow push .../.ruff_cache` commands.
        return sorted(p for p in wm.iterdir()
                      if p.is_dir() and (p / "flow.yaml").is_file())
    if engine == "dagu":
        return sorted((cdir / "automation" / "dagu").glob("*.yaml"))
    if engine == "retrieval":
        return sorted((cdir / "automation" / "retrieval").glob("*.go"))
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
    elif engine == "dagu":
        lines += [
            "# install dagu (single binary, no DB — docs/script-execution.md)",
            "curl -fsSL https://raw.githubusercontent.com/dagucloud/dagu/main/scripts/installer.sh | bash",
        ]
        for path in defs:
            try:
                rel = path.relative_to(root)
            except ValueError:
                rel = path
            lines.append('dagu validate "%s"' % rel)
        dags_dir = "clients/%s/automation/dagu" % client
        lines += [
            "# run scheduler and UI pointing at this client's DAG directory",
            'dagu start-all --dags "%s" &' % dags_dir,
        ]
    elif engine == "windmill":
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
    else:
        lines += [
            "# unknown engine: %s" % engine,
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

    uses_nango = False
    if env_example.is_file() and ("NANGO_" in env_example.read_text()):
        uses_nango = True
    if not uses_nango:
        for p in defs:
            try:
                txt = p.read_text().lower()
                if "nango" in txt or "provider-config-key" in txt:
                    uses_nango = True
                    break
            except Exception:
                pass

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
    if uses_nango:
        add("")
        add("### SaaS Auth Layer (Nango)")
        add("This client routes SaaS requests via Nango (`Connection-Id: %s`)." % client)
        add("1. On NEW server: bootstrap dedicated Nango with `./scripts/setup-nango.sh --up`")
        add("   (or point `NANGO_HOST` to the client's isolated Nango instance).")
        add("2. Authorize connections for this client in Nango. Either create them with `connection_id: %s` (headless auth / import), or use a connect link and record the id Nango assigned in `clients/%s/nango/connections.json`." % (client, client))
        add("3. Set `NANGO_HOST` and `NANGO_SECRET_KEY` on the new host.")

    policy = cdir / "catalog" / "policy.yaml"
    access = access_block(cdir / "client.yaml")
    if (access.get("enabled") or "").lower() == "true":
        plane = access.get("plane") or "?"
        identity = access.get("identity") or "org"
        add("")
        add("### Access plane (the client's agents calling tools)")
        add("Broker plane **%s**, identity **%s** (docs/access-plane.md)." % (plane, identity))
        if policy.is_file():
            add("- Ceiling to carry as-is: `%s`" % policy.relative_to(root))
        add("1. Start this client's broker: `NANGO_CLIENT_ID=%s NANGO_PORT=8188 NANGO_CONNECT_PORT=8189 "
            "./scripts/setup-nango.sh --up`." % client)
        add("2. Move the connections: they are rows in the broker's own Postgres "
            "(volume `nango-%s_nango-db-data`). Dump and restore, or re-authorise — "
            "only the client can consent." % client)
        add("3. Recreate the grants in the new broker for tenant `%s`, then revoke them "
            "on the old one; revocation is a row delete, not a deploy." % client)
        if plane != "client":
            add("4. **The manifest says `plane: %s`** — an extracted client is "
                "client-hosted, so set `access.plane: client` before cutover "
                "(a sensitive client on any other plane fails `go test ./clients/`)." % plane)
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
    elif engine == "dagu":
        add("```bash")
        for n in names:
            add("dagu stop %s" % n)
        add("# or remove/archive DAG YAMLs from the old DAGS directory")
        add("```")
    elif engine == "windmill":
        add("Disable only this client's workspace schedules; leave other workspaces alone.")
    else:
        add("Disable this client on the old server.")
    if uses_nango:
        add("")
        add("Purge this client's connections from the shared Nango to prevent lingering tokens:")
        add("```bash")
        add('# curl -X DELETE -H "Authorization: Bearer $SHARED_NANGO_SECRET_KEY" \\')
        add('#   "$SHARED_NANGO_HOST/connections/%s?provider_config_key=<provider>"' % client)
        add("```")
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
