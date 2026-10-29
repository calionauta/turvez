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

Writes:
  promptfoo/cases.json      tests ready for `tests: file://cases.json`
  promptfoo/selection.json  audit trail: why each run was selected

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

GATE_STEP = "gate"
LLM_STEP = "triage"
CATEGORIES = ["billing", "technical", "other"]


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


def classify(run: dict) -> list[str]:
    """Return the reasons this run is worth a case. Empty = boring, skip it."""
    reasons: list[str] = []
    if run.get("status") == "failed":
        reasons.append("run_failed")
    for step_id, state in run.get("steps", {}).items():
        if state.get("status") == "failed":
            reasons.append(f"step_failed:{step_id}")
        if (state.get("attempts") or 0) > 1:
            reasons.append(f"retried:{step_id}")
    gate = step_output(run, GATE_STEP)
    if isinstance(gate, dict) and gate.get("matched") is False:
        reasons.append("gate_rejected")
    llm = step_output(run, LLM_STEP)
    if isinstance(llm, dict):
        content = (llm.get("content") or "").strip()
        try:
            parsed = json.loads(content)
        except json.JSONDecodeError:
            reasons.append("output_not_json")
        else:
            if not isinstance(parsed, dict) or parsed.get("category") not in CATEGORIES:
                reasons.append("output_bad_schema")
    return reasons


def load_runs(args) -> list[dict]:
    if args.dagnats:
        listing = json.loads(subprocess.check_output(["dagnats", "run", "list", "--json"], text=True))
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


def build_case(run: dict, reasons: list[str]) -> dict | None:
    # The prompt is the only hard requirement: a failed call is still replayable
    # when the prompt was emitted by a step (that is why the recipe builds it in
    # `build-prompt` instead of inlining it in `llm.complete` metadata).
    prompt = step_output(run, "build-prompt")
    llm = step_output(run, LLM_STEP)
    if not isinstance(prompt, dict):
        return None

    messages = prompt.get("messages") or []
    user = next((m.get("content", "") for m in messages if m.get("role") == "user"), "")
    system = next((m.get("content", "") for m in messages if m.get("role") == "system"), "")
    actual = (llm.get("content") or "").strip() if isinstance(llm, dict) else None
    digest = hashlib.sha256((system + "\x00" + user + "\x00" + (actual or "")).encode()).hexdigest()[:16]

    return {
        "description": f"{run['run_id']} [{','.join(reasons)}]",
        "vars": {"text": user, "ticket_id": (run.get("input") or {}).get("ticket_id", "")},
        "assert": [
            {
                "type": "is-json",
                "value": {
                    "type": "object",
                    "required": ["category", "urgency"],
                    "properties": {
                        "category": {"enum": CATEGORIES},
                        "urgency": {"enum": ["low", "high"]},
                    },
                },
            },
            {
                # The rubric must carry the input: the grader only sees what the
                # assertion value renders. Without {{text}} it grades blind.
                "type": "llm-rubric",
                "value": "Ticket: {{text}}\n\nThe JSON category must match the "
                         "ticket's actual intent (billing, technical or other) and the "
                         "urgency must be defensible.",
            },
        ],
        "metadata": {
            "run_id": run["run_id"],
            "reasons": reasons,
            "hash": digest,
            "production_output": actual,
            "expected_category": None,  # fill by hand when you label the case
            "system_prompt": system,
        },
    }


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--runs", help="inspect-shaped JSON file")
    ap.add_argument("--dagnats", action="store_true", help="query the live dagnats CLI")
    ap.add_argument("--out", default="promptfoo", help="output dir")
    ap.add_argument("--include-ok", type=int, default=0,
                    help="also take N clean runs as control cases (deterministic order)")
    args = ap.parse_args()
    if not args.runs and not args.dagnats:
        ap.error("pass --runs FILE or --dagnats")

    runs = load_runs(args)
    cases, seen, audit = [], set(), []
    controls = args.include_ok

    for run in runs:
        reasons = classify(run)
        if not reasons and controls > 0:
            reasons = ["control"]
            controls -= 1
        if not reasons:
            continue
        case = build_case(run, reasons)
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

    out = Path(args.out)
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
