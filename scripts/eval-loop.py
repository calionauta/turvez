#!/usr/bin/env python3
"""Run the whole eval loop for one client: ingest, label, evaluate, summarize.

    python3 scripts/eval-loop.py --dir clients/acme/evals

Steps, in order:
  1. read run history and regenerate cases (read-only)
  2. label the unlabeled cases, when stdin is a terminal
  3. run promptfoo, after confirmation
  4. print pass rate, assertion failures and per-field accuracy

Flags exist to stop after any step. The per-client taxonomy lives in
`eval.config.json`; this script has no client-specific code.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from collections import Counter
from pathlib import Path

import evalconfig

SCRIPTS = Path(__file__).resolve().parent


def run(cmd: list[str], cwd: Path | None = None) -> None:
    subprocess.run(cmd, cwd=str(cwd) if cwd else None, check=True)


def select_cases(root: Path, include_ok: int, dagnats: bool = True,
                 strategy: str = "targeted", seed: int = 0, limit: int = 0) -> None:
    cmd = [
        sys.executable, str(SCRIPTS / "select-cases.py"),
        "--out", str(root / "cases"),
        "--include-ok", str(include_ok),
        "--strategy", strategy,
        "--seed", str(seed),
        "--limit", str(limit),
    ]
    if dagnats:
        cmd.append("--dagnats")
    else:
        cmd.append("--runs")
        cmd.append(str(root / "cases" / "runs.json"))
    run(cmd)


def label_cases(root: Path) -> None:
    run([sys.executable, str(SCRIPTS / "label-cases.py"), "--dir", str(root)])


def run_eval(root: Path, cfg: dict) -> None:
    cmd = [
        "npx", "-y", "promptfoo@latest", "eval",
        "-c", "promptfooconfig.yaml",
        "--no-cache",
        "-o", "results.json",
    ]
    judge = (cfg.get("judge") or {}).get("provider")
    if cfg.get("rubric") and judge:
        cmd += ["--grader", judge]
    elif cfg.get("rubric") and not judge:
        print("warning: rubric is enabled but judge.provider is empty; "
              "promptfoo will use its default grader", flush=True)
    run(cmd, cwd=root)


def summarize(root: Path, cfg: dict, strict: bool) -> int:
    path = root / "results.json"
    if not path.exists():
        print(f"no {path}; the eval did not produce results")
        return 1
    doc = json.loads(path.read_text())
    rows = doc.get("results", {}).get("results", [])
    stats = doc.get("results", {}).get("stats", {})
    passed = stats.get("successes", sum(1 for r in rows if r.get("success")))
    failed = stats.get("failures", len(rows) - passed)

    print(f"\ncases {len(rows)} · pass {passed} · fail {failed}")

    breaks: Counter[str] = Counter()
    for row in rows:
        for comp in (row.get("gradingResult", {}) or {}).get("componentResults", []) or []:
            if not comp.get("pass"):
                breaks[comp.get("assertion", {}).get("type", "?")] += 1
    if breaks:
        print("failing assertions: " + ", ".join(f"{k} {v}" for k, v in breaks.items()))

    if evalconfig.retrieval_enabled(cfg):
        print("\nretrieval fields:")
        for field in evalconfig.retrieval_fields(cfg):
            _print_labeled_accuracy(rows, field)
        print(f"\nresults: {path}")
        print("view:   make eval-client-view CLIENT=<client> TURVEZ_OPS_DIR=...")
        if strict and (failed or breaks):
            return 1
        return 0

    for field in evalconfig.fields(cfg):
        var, name = field["var"], field["name"]
        labeled = [r for r in rows if r.get("testCase", {}).get("vars", {}).get(var)]
        if not labeled:
            continue
        ok = 0
        for row in labeled:
            try:
                output = json.loads(row.get("response", {}).get("output") or "")
            except json.JSONDecodeError:
                continue
            if isinstance(output, dict) and output.get(name) == row["testCase"]["vars"][var]:
                ok += 1
        pct = (ok / len(labeled)) * 100
        print(f"{name}: {ok}/{len(labeled)} ({pct:.0f}%)")

    print(f"\nresults: {path}")
    print("view:   make eval-client-view CLIENT=<client> TURVEZ_OPS_DIR=...")
    if strict and (failed or breaks):
        return 1
    return 0


def _print_labeled_accuracy(rows: list[dict], field: dict) -> None:
    name, var = field["name"], field["var"]
    labeled = [r for r in rows if r.get("testCase", {}).get("vars", {}).get(var)]
    print(f"  {name}: {len(labeled)} labeled")
    if not labeled:
        return
    counts: Counter[str] = Counter()
    for row in labeled:
        try:
            output = json.loads(row.get("response", {}).get("output") or "")
        except json.JSONDecodeError:
            counts["(unparseable)"] += 1
            continue
        if isinstance(output, dict) and name in output:
            counts[output[name]] += 1
        else:
            counts["(missing)"] += 1
    for value in field["values"]:
        print(f"    {value!r}: {counts.get(value, 0)}")


def main() -> int:
    ap = argparse.ArgumentParser(
        description="Ingest run history, label, evaluate and summarize one client.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""examples:
  python3 scripts/eval-loop.py --dir clients/acme/evals
  python3 scripts/eval-loop.py --dir clients/acme/evals --skip-label --yes
  python3 scripts/eval-loop.py --dir clients/acme/evals --skip-eval
  python3 scripts/eval-loop.py --dir clients/acme/evals --strict

requires DAGNATS_NATS_URL to point at the client's stack.
see skills/turvez/references/eval-loop.md.""")
    ap.add_argument("--dir", required=True, help="client eval dir (has cases/ and eval.config.json)")
    ap.add_argument("--include-ok", type=int, default=3, help="clean control cases to add")
    ap.add_argument("--strategy", choices=["targeted", "random", "diverse", "stratified"],
                    default="targeted", help="select-cases.py sampling strategy")
    ap.add_argument("--seed", type=int, default=0, help="seed for random/diverse/stratified")
    ap.add_argument("--limit", type=int, default=0, help="cap on generated cases (0 = no cap)")
    ap.add_argument("--skip-label", action="store_true", help="do not label in this run")
    ap.add_argument("--skip-eval", action="store_true", help="stop after generating cases")
    ap.add_argument("--yes", action="store_true", help="skip the eval confirmation")
    ap.add_argument("--strict", action="store_true", help="exit 1 when any case fails")
    ap.add_argument("--dagnats", action="store_true", default=True,
                    help="read live dagnats run history (default)")
    ap.add_argument("--no-dagnats", action="store_true",
                    help="use the frozen runs.json instead of the live CLI")
    args = ap.parse_args()
    args.dagnats = args.dagnats and not args.no_dagnats

    root = Path(args.dir)
    cfg_path = evalconfig.default_path(root)
    if not cfg_path.exists():
        print(f"no {cfg_path}; run: python3 scripts/label-cases.py --dir {root} --setup")
        return 1
    cfg = evalconfig.load(cfg_path)

    select_cases(root, args.include_ok, dagnats=args.dagnats,
                 strategy=args.strategy, seed=args.seed, limit=args.limit)
    if args.skip_eval:
        return 0

    if args.skip_label:
        print("labeling: skipped (--skip-label)", flush=True)
    elif sys.stdin.isatty():
        label_cases(root)
    else:
        print("labeling: skipped (stdin is not a terminal; run label-cases.py manually)", flush=True)

    if not args.yes and sys.stdin.isatty():
        answer = input("run the eval? judge tokens may be spent [y/N]: ").strip().lower()
        if answer not in ("y", "yes"):
            print("eval: skipped")
            return 0

    run_eval(root, cfg)
    return summarize(root, cfg, args.strict)

if __name__ == "__main__":
    sys.exit(main())
