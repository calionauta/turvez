#!/usr/bin/env python3
"""Tests for select-cases.py sampling strategies (targeted/random/diverse/stratified).

Builds a synthetic runs file, runs select-cases.py as a subprocess per
strategy, and asserts on the resulting cases.json + selection.json.

Usage: python3 scripts/test-select-cases.py
"""
from __future__ import annotations

import base64
import hashlib
import json
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))

CFG = {
    "prompt_step": "build-prompt",
    "result_step": "triage",
    "gate_step": "gate",
    "text_var": "text",
    "text_role": "user",
    "fields": [{"name": "category", "var": "expected_category",
                "values": ["billing", "technical", "other"]}],
    "schema_extra": {},
    "rubric": "",
}


def b64(payload) -> str:
    return base64.b64encode(json.dumps(payload).encode()).decode()


def make_run(run_id: str, text: str, bad: bool = False, status: str = "completed") -> dict:
    steps = {
        "build-prompt": {"status": "completed", "output": b64(
            {"messages": [{"role": "system", "content": "SYS"},
                          {"role": "user", "content": text}]})},
        "triage": {"status": "completed",
                   "output": b64({"content": json.dumps({"category": "billing"})})},
        "gate": {"status": "completed", "output": b64({"matched": True})},
    }
    if bad:
        steps["triage"] = {"status": "failed", "output": b64({"content": "not json"})}
        status = "failed"
    return {"run_id": run_id, "status": status, "steps": steps}


def run_select(runs: list[dict], strategy: str, seed: int = 0, limit: int = 0,
               include_ok: int = 0) -> tuple[list, dict]:
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        runs_path = root / "runs.json"
        runs_path.write_text(json.dumps({"runs": runs}))
        (root / "eval.config.json").write_text(json.dumps(CFG))
        out = root / "cases"
        cmd = [sys.executable, str(ROOT / "scripts/select-cases.py"),
               "--runs", str(runs_path), "--out", str(out),
               "--strategy", strategy, "--seed", str(seed),
               "--limit", str(limit), "--include-ok", str(include_ok),
               "--config", str(root / "eval.config.json")]
        proc = subprocess.run(cmd, capture_output=True, text=True)
        assert proc.returncode == 0, f"{strategy}: {proc.stderr}"
        cases = json.loads((out / "cases.json").read_text())
        selection = json.loads((out / "selection.json").read_text())
        assert selection["strategy"] == strategy
        return cases, selection


def ids(cases: list) -> list[str]:
    return [c["metadata"]["run_id"] for c in cases]


def main() -> int:
    fails: list[str] = []

    def check(name: str, cond: bool, detail: str = "") -> None:
        print(f"{'ok' if cond else 'FAIL'}: {name}" + (f" — {detail}" if not cond else ""))
        if not cond:
            fails.append(name)

    clean = [make_run(f"r{i}", f"invoice charge question {i}", bad=False) for i in range(1, 7)]
    failed = make_run("bad1", "broken parser", bad=True)
    runs = clean + [failed]

    # targeted: signal-first, deterministic, backward compatible
    cases, sel = run_select(runs, "targeted", include_ok=2)
    check("targeted selects failed run", "bad1" in ids(cases), str(ids(cases)))
    check("targeted takes first-N clean controls", ids(cases)[:2] == ["r1", "r2"], str(ids(cases)))
    check("targeted dedupes", len(cases) == len(set(ids(cases))))

    # targeted reproducibility (already the contract today)
    cases2, _ = run_select(runs, "targeted", include_ok=2)
    check("targeted reproducible", ids(cases) == ids(cases2))

    # random: seeded, full pool, limit respected
    cases_r, sel_r = run_select(runs, "random", seed=7, limit=4)
    check("random respects limit", len(cases_r) == 4, str(ids(cases_r)))
    cases_r2, _ = run_select(runs, "random", seed=7, limit=4)
    check("random reproducible per seed", ids(cases_r) == ids(cases_r2))
    cases_r3, _ = run_select(runs, "random", seed=8, limit=4)
    check("random varies per seed", ids(cases_r) != ids(cases_r3), "seeds 7/8 same order")
    check("random tags reasons", any("sample:random" in str(s.get("reasons", ""))
                                      for s in sel_r["audit"]), str(sel_r["audit"][:1]))

    # diverse: greedy coverage — duplicate near-identical texts collapse
    dup = [make_run("d1", "the cat sat on the mat"), make_run("d2", "the cat sat on the mat"),
           make_run("d3", "quantum chromodynamics equations derive lattice gauge"),
           make_run("d4", "refund policy for broken items")]
    cases_d, _ = run_select(dup, "diverse", limit=3)
    check("diverse dedupes identical inputs", len(cases_d) == 3, str(ids(cases_d)))
    check("diverse avoids the duplicate", "d1" not in ids(cases_d) or "d2" not in ids(cases_d))

    # stratified: every stratum represented
    strat = [make_run(f"s{i}", f"text {i}", bad=(i % 3 == 0)) for i in range(1, 7)]
    cases_s, sel_s = run_select(strat, "stratified", limit=6)
    check("stratified labels strata in reasons",
          any("sample:stratified:" in str(s.get("reasons", "")) for s in sel_s["audit"]))
    bad_ids = {r["run_id"] for r in strat if r["status"] == "failed"}
    check("stratified includes failures", bool(bad_ids & set(ids(cases_s))), str(ids(cases_s)))

    if fails:
        print(f"test-select-cases: {len(fails)} failure(s)", file=sys.stderr)
        return 1
    print("test-select-cases: ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
