#!/usr/bin/env python3
"""Select eval candidates from dagnats run history (metadata-first).

Why this exists: production runs are not ground truth, and a random sample is
the least informative slice of them. This picks the runs that carry a signal —
failures, retries, gate rejections, malformed output — deduplicates them, and
freezes the selection with a content hash so an eval is reproducible.

Reads either:
  --runs FILE      a JSON file shaped like `dagnats run inspect <id> --json`,
                   or {"runs": [ ... those ... ]}
  --dagnats        live: `dagnats run list --json` then inspect each run

The client's eval.config.json says which steps hold the prompt/result and which
fields the operator labels. Without it, the support-ticket example is used.

Writes:
  <out>/cases.json      tests ready for `tests: file://cases/cases.json`
  <out>/selection.json  audit trail: why each run was selected

No prompt content is required to *select*: selection uses status, attempts,
step errors and the deterministic gate. Content is only materialised for the
runs that were already picked, and lives next to the run id so it can be
traced back — redact before committing, never commit the raw file.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

import evalconfig


def decode(value: str | None):
    """Step outputs are base64 of the raw JSON payload, "" when skipped."""
    if not value:
        return None
    raw = base64.b64decode(value)
    if not raw:
        return None
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        return raw.decode("utf-8", "replace")


def step_output(run: dict, step: str):
    state = run.get("steps", {}).get(step) or {}
    return decode(state.get("output"))


def classify(run: dict, cfg: dict) -> list[str]:
    """Return the reasons this run is worth a case. Empty = boring, skip it."""
    reasons: list[str] = []
    if run.get("status") == "failed":
        reasons.append("run_failed")
    for step_id, state in run.get("steps", {}).items():
        if state.get("status") == "failed":
            reasons.append(f"step_failed:{step_id}")
        if (state.get("attempts") or 0) > 1:
            reasons.append(f"retried:{step_id}")

    gate = step_output(run, cfg["gate_step"])
    if isinstance(gate, dict) and gate.get("matched") is False:
        reasons.append("gate_rejected")

    llm = step_output(run, cfg["result_step"])
    if isinstance(llm, dict):
        content = (llm.get("content") or "").strip()
        try:
            parsed = json.loads(content)
        except json.JSONDecodeError:
            reasons.append("output_not_json")
        else:
            if not evalconfig.matches_schema(parsed, cfg):
                reasons.append("output_bad_schema")
    return reasons


def load_runs(args) -> list[dict]:
    if args.dagnats:
        listing = json.loads(subprocess.check_output(
            ["dagnats", "run", "list", "--json"], text=True))
        runs = listing.get("runs", listing) if isinstance(listing, dict) else listing
        out = []
        for item in runs:
            run_id = item.get("run_id") or item.get("id")
            if not run_id:
                continue
            out.append(json.loads(subprocess.check_output(
                ["dagnats", "run", "inspect", run_id, "--json"], text=True))["run"])
        return out

    doc = json.loads(Path(args.runs).read_text())
    runs = doc["runs"] if isinstance(doc, dict) and "runs" in doc else doc
    # Accept both `[{"run": {...}}]` (what the CLI emits) and `[{...}]`.
    return [item.get("run", item) for item in runs]


def build_case(run: dict, reasons: list[str], labels: dict, cfg: dict) -> dict | None:
    # The prompt is the only hard requirement: a failed call is still replayable
    # when the prompt was emitted by a step (that is why the recipe builds it in
    # a step instead of inlining it in `llm.complete` metadata).
    prompt = step_output(run, cfg["prompt_step"])
    llm = step_output(run, cfg["result_step"])
    if not isinstance(prompt, dict):
        return None

    messages = prompt.get("messages") or []
    role = cfg.get("text_role", "user")
    user = next((m.get("content", "") for m in messages if m.get("role") == role), "")
    system = next((m.get("content", "") for m in messages if m.get("role") == "system"), "")
    actual = (llm.get("content") or "").strip() if isinstance(llm, dict) else None
    input_digest = hashlib.sha256((system + "\x00" + user).encode()).hexdigest()[:16]
    digest = hashlib.sha256(
        (system + "\x00" + user + "\x00" + (actual or "")).encode()).hexdigest()[:16]

    # Labels key on the input, not the case: the same input may appear twice
    # (a good run and a failed one) and both should inherit the same label.
    label = labels.get(input_digest) or {}
    case = {
        "description": f"{run['run_id']} [{','.join(reasons)}]",
        "vars": {cfg.get("text_var", "text"): user},
        "assert": [{"type": "is-json", "value": evalconfig.schema(cfg)}],
        "metadata": {
            "run_id": run["run_id"],
            "reasons": reasons,
            "hash": digest,
            "input_hash": input_digest,
            "production_output": actual,
            "system_prompt": system,
            "expected": {},
        },
    }
    if cfg.get("rubric"):
        case["assert"].append({"type": "llm-rubric", "value": cfg["rubric"]})

    for field in evalconfig.fields(cfg):
        value = label.get(field["var"])
        if not value:
            continue
        case["vars"][field["var"]] = value
        case["metadata"]["expected"][field["name"]] = value
        case["assert"].append({
            "type": "javascript",
            # Single expression: promptfoo wraps the value, so a `return` body fails.
            "value": f"JSON.parse(output)[{json.dumps(field['name'])}] === "
                     + json.dumps(value),
        })
    return case


def main() -> int:
    ap = argparse.ArgumentParser(
        description="Select eval cases from dagnats run history.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""examples:
  python3 scripts/select-cases.py --dagnats --out evals/acme/cases --include-ok 3
  python3 scripts/select-cases.py --runs run.json --out /tmp/cases
  python3 scripts/select-cases.py --help

the client config (evals/<client>/eval.config.json) supplies step names,
label fields and schema. see skills/turvez/references/eval-loop.md.""")
    ap.add_argument("--runs", help="inspect-shaped JSON file")
    ap.add_argument("--dagnats", action="store_true", help="query the live dagnats CLI")
    ap.add_argument("--out", default="promptfoo", help="output dir")
    ap.add_argument("--config", default=None,
                    help="eval.config.json (default: <out>/../eval.config.json)")
    ap.add_argument("--include-ok", type=int, default=0,
                    help="also take N clean runs as control cases (deterministic order)")
    ap.add_argument("--labels", default=None,
                    help="labels.json keyed by case hash (default: <out>/../labels.json)")
    args = ap.parse_args()
    if not args.runs and not args.dagnats:
        ap.error("pass --runs FILE or --dagnats")

    out = Path(args.out)
    cfg_path = Path(args.config) if args.config else evalconfig.default_path(out.parent)
    cfg = evalconfig.load(cfg_path)
    if cfg_path.exists():
        print(f"config: {cfg_path}")
    else:
        print(f"config: defaults (no {cfg_path})")

    labels_path = Path(args.labels) if args.labels else out.parent / "labels.json"
    labels = {}
    if labels_path.exists():
        labels = json.loads(labels_path.read_text())
        print(f"labels: {len(labels)} entr{'y' if len(labels)==1 else 'ies'} from {labels_path}")

    runs = load_runs(args)
    cases, seen, audit = [], set(), []
    controls = args.include_ok

    for run in runs:
        reasons = classify(run, cfg)
        if not reasons and controls > 0:
            reasons = ["control"]
            controls -= 1
        if not reasons:
            continue
        case = build_case(run, reasons, labels, cfg)
        if case is None:
            audit.append({"run_id": run["run_id"], "reasons": reasons, "case": None,
                          "why": "no prompt/output recorded (failed call or inline prompt)"})
            continue
        digest = case["metadata"]["hash"]
        if digest in seen:
            audit.append({"run_id": run["run_id"], "reasons": reasons, "hash": digest,
                          "why": "duplicate of an earlier case"})
            continue
        seen.add(digest)
        cases.append(case)
        audit.append({"run_id": run["run_id"], "reasons": reasons, "hash": digest,
                      "why": "selected"})

    out.mkdir(parents=True, exist_ok=True)
    (out / "cases.json").write_text(json.dumps(cases, indent=2) + "\n")
    (out / "selection.json").write_text(json.dumps({
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "runs_examined": len(runs),
        "cases": len(cases),
        "audit": audit,
    }, indent=2) + "\n")

    print(f"examined {len(runs)} runs -> {len(cases)} cases -> {out}/cases.json")
    for entry in audit:
        print(f"  {entry['run_id']:<18} {','.join(entry['reasons']):<28} {entry['why']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
