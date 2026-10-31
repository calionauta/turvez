#!/usr/bin/env python3
"""Convert a Windmill flow job into the `dagnats run inspect` shape.

Reads `wmill job get <flow-job-id> --json`, fetches each module job result with
`wmill job get <job-id> --json`, and writes a normalized run that
`select-cases.py --runs` can read. Step outputs are base64 JSON, matching the
dagnats shape, so no other change is needed.

    python3 scripts/windmill-job-to-run.py --job-id <id> --out run.json
    python3 scripts/windmill-job-to-run.py --job flow.json --out run.json
"""

import argparse
import base64
import json
import subprocess
import sys


def wmill_job(job_id):
    raw = subprocess.check_output(
        ["wmill", "job", "get", job_id, "--json"], text=True)
    return json.loads(raw)


def b64(value):
    if value is None:
        return ""
    raw = json.dumps(value, sort_keys=True).encode()
    return base64.b64encode(raw).decode()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--job-id", help="flow job id (uses wmill CLI)")
    ap.add_argument("--job", help="flow job JSON file")
    ap.add_argument("--out", required=True)
    args = ap.parse_args()
    if not args.job_id and not args.job:
        ap.error("pass --job-id or --job")

    flow = wmill_job(args.job_id) if args.job_id else json.loads(
        open(args.job).read())

    steps = {}
    for module in flow.get("flow_status", {}).get("modules", []):
        step_id = module.get("id")
        if not step_id:
            continue
        status = "skipped" if module.get("skipped") else (
            "completed" if module.get("type") == "Success" else "failed")
        output = None
        job_id = module.get("job")
        if job_id and status != "skipped":
            output = wmill_job(job_id).get("result")
        steps[step_id] = {
            "status": status,
            "attempts": 1,
            "output": b64(output),
        }

    run = {
        "run_id": flow.get("id"),
        "workflow_id": flow.get("script_path"),
        "status": "completed" if flow.get("success") else "failed",
        "input": flow.get("args") or {},
        "created_at": flow.get("created_at"),
        "completed_at": flow.get("completed_at"),
        "steps": steps,
    }
    with open(args.out, "w") as fh:
        json.dump({"run": run}, fh, indent=2)
    print(f"normalized {len(steps)} step(s) -> {args.out}")


if __name__ == "__main__":
    sys.exit(main())
