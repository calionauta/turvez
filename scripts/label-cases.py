#!/usr/bin/env python3
"""Label eval cases by exception.

Most production outputs are already right, so the fast path is: accept the
suggested value, override only the wrong ones. Fields, values and shortcuts
come from the client's eval.config.json, so the same script works for any
classification. Labels are keyed by the case hash and written to labels.json,
which survives case regeneration.

Usage:
  python3 scripts/label-cases.py --dir clients/acme/evals
  python3 scripts/label-cases.py --dir clients/acme/evals --setup   # define fields
  python3 scripts/label-cases.py --dir clients/acme/evals --blind   # hide suggestions

Keys: enter accepts the suggestion, a configured shortcut picks that value,
1..N picks by position, any other text is stored as a free value, s skips the
case, q quits (progress is saved after each case).
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import evalconfig


def ask(prompt: str) -> str:
    try:
        return input(prompt).strip()
    except EOFError:
        return "q"


def suggested(production_output: str | None, field: dict) -> str | None:
    try:
        parsed = json.loads(production_output or "")
    except json.JSONDecodeError:
        return None
    if not isinstance(parsed, dict):
        return None
    value = parsed.get(field["name"])
    return value if value in field["values"] else None


def setup(cfg_path: Path, cfg: dict) -> None:
    print("Define the label fields. Enter keeps the current value.")
    current = ",".join(f["name"] for f in cfg["fields"])
    names = ask(f"fields (comma-separated) [{current}]: ")
    if not names:
        cfg_path.write_text(json.dumps(cfg, indent=2) + "\n")
        print(f"wrote {cfg_path}")
        return

    fields = []
    for name in [n.strip() for n in names.split(",") if n.strip()]:
        existing = next((f for f in cfg["fields"] if f["name"] == name), None)
        default_values = ",".join(existing["values"]) if existing else ""
        raw = ask(f"  values for {name} [{default_values}]: ") or default_values
        values = [v.strip() for v in raw.split(",") if v.strip()]
        if not values:
            print(f"  {name}: no values, skipped")
            continue
        fields.append({
            "name": name,
            "var": f"expected_{name}",
            "values": values,
        })
    cfg["fields"] = fields
    cfg_path.write_text(json.dumps(cfg, indent=2) + "\n")
    print(f"wrote {cfg_path}")


def prompt_field(field: dict, suggestion: str | None, blind: bool) -> str | None:
    keys = evalconfig.shortcuts(field)
    values = field["values"]
    options = ", ".join(f"{k}={v}" for k, v in keys.items())
    options += f", 1-{len(values)}"
    hint = "?" if blind or not suggestion else suggestion
    while True:
        raw = ask(f"  {field['name']} [{options}, enter={hint}, text, skip, quit]: ")
        if raw in ("q", "quit"):
            return "quit"
        if raw in ("s", "skip"):
            return None
        if raw == "":
            if suggestion and not blind:
                return suggestion
            print("  no suggestion, pick a key, a number or type a value")
            continue
        if raw in keys:
            return keys[raw]
        if raw.isdigit() and 1 <= int(raw) <= len(values):
            return values[int(raw) - 1]
        return raw


def main() -> int:
    ap = argparse.ArgumentParser(
        description="Label eval cases by exception, using the client's taxonomy.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""keys:
  enter      accept the production suggestion
  b / t / o  pick a configured shortcut (unique prefixes, see eval.config.json)
  1..N       pick by position
  any text   stored as a new value and appended to eval.config.json
  skip       skip this case (s also works)
  quit       quit, progress is saved (q also works)

examples:
  python3 scripts/label-cases.py --dir clients/acme/evals --setup
  python3 scripts/label-cases.py --dir clients/acme/evals
  python3 scripts/label-cases.py --dir clients/acme/evals --blind

see skills/turvez/references/eval-loop.md.""")
    ap.add_argument("--dir", required=True, help="client eval dir (contains cases/)")
    ap.add_argument("--config", default=None,
                    help="eval.config.json (default: <dir>/eval.config.json)")
    ap.add_argument("--labels", default=None, help="default: <dir>/labels.json")
    ap.add_argument("--setup", action="store_true", help="define fields and values, then exit")
    ap.add_argument("--blind", action="store_true", help="hide production suggestions")
    args = ap.parse_args()

    root = Path(args.dir)
    cfg_path = Path(args.config) if args.config else evalconfig.default_path(root)
    cfg = evalconfig.load(cfg_path)
    if args.setup:
        setup(cfg_path, cfg)
        return 0
    if not cfg_path.exists():
        print(f"no {cfg_path}; run with --setup to define the fields")

    if not evalconfig.fields(cfg):
        print("no label fields configured; nothing to label")
        return 0

    cases_path = root / "cases" / "cases.json"
    labels_path = Path(args.labels) if args.labels else root / "labels.json"
    cases = json.loads(cases_path.read_text())
    labels = json.loads(labels_path.read_text()) if labels_path.exists() else {}

    def label_key(case: dict) -> str:
        # Labels key on the input so a rerun with a different output keeps them.
        return case["metadata"].get("input_hash") or case["metadata"]["hash"]

    def complete(case: dict) -> bool:
        entry = labels.get(label_key(case)) or {}
        return all(entry.get(f["var"]) for f in evalconfig.fields(cfg))

    todo = [c for c in cases if not complete(c)]
    if not todo:
        print(f"all {len(cases)} case(s) already labeled in {labels_path}")
        return 0

    names = ", ".join(f["name"] for f in evalconfig.fields(cfg))
    print(f"{len(todo)} unlabeled case(s) of {len(cases)}. fields: {names}.")
    print("enter accepts, keys pick, text is stored as-is, skip/quit exit a case or the run.\n")

    for i, case in enumerate(todo, 1):
        digest = label_key(case)
        text = (case.get("vars", {}).get("text") or "").strip().replace("\n", " ")
        print(f"[{i}/{len(todo)}] {case['metadata'].get('run_id')}  {digest}")
        print(f"  text: {text[:180]}{'…' if len(text) > 180 else ''}")

        entry = dict(labels.get(digest) or {})
        quit_now = False
        for field in evalconfig.fields(cfg):
            if entry.get(field["var"]):
                continue
            suggestion = suggested(case["metadata"].get("production_output"), field)
            value = prompt_field(field, suggestion, args.blind)
            if value == "quit":
                quit_now = True
                break
            if value is not None:
                entry[field["var"]] = value
                if value not in field["values"]:
                    # A new classification typed by the operator becomes part
                    # of the taxonomy; the next case generation uses it.
                    field["values"].append(value)
                    cfg_path.write_text(json.dumps(cfg, indent=2) + "\n")
                    print(f"  added {value!r} to {field['name']} values")

        if entry:
            labels[digest] = entry
            labels_path.write_text(json.dumps(labels, indent=2) + "\n")
        print()
        if quit_now:
            break

    labels_path.write_text(json.dumps(labels, indent=2) + "\n")
    print(f"labels: {len(labels)} total in {labels_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
