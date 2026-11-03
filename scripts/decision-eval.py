#!/usr/bin/env python3
"""decision-eval.py — behavioral eval for the decision tree.

The deterministic contract in scripts/test-decisions.sh proves every branch
names an artifact that exists. It cannot prove that an agent reads the manifest
before asking, asks at most three branch-flipping questions, puts a
recommendation on each one, or records the decision. This script measures that
with operator scenarios and a transcript grader.

CI runs --check --self-test; it never calls a model. A model run is local:
produce a transcript with any agent, then --grade it. How to produce one:
skills/turvez/evals/README.md.
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[1]
SCENARIOS = ROOT / "skills/turvez/evals/scenarios.yaml"
ENGINES = ROOT / "engines.json"

RUNTIMES = {"shared", "dedicated"}
LEVELS = {"0", "1", "2", "3"}
CAPABILITIES = {"none", "genkit", "retrieval", "sandbox"}
LIBRARIES = {"none", "stdlib", "existing", "approved", "new-confirmed", "refuse"}
EXPECT_KEYS = {"engine", "runtime", "level", "capability", "library", "record"}


def norm(value: object) -> str:
    return str(value).strip().lower()


def load_scenarios() -> list[dict]:
    data = yaml.safe_load(SCENARIOS.read_text())
    if not isinstance(data, list) or not data:
        raise ValueError(f"{SCENARIOS} must be a non-empty YAML list")
    return data


def load_engines() -> set[str]:
    return set(json.loads(ENGINES.read_text())["engines"])


def check_scenarios(scenarios: list[dict], engines: set[str]) -> list[str]:
    errors: list[str] = []
    seen_ids: set[str] = set()
    seen_engines: set[str] = set()

    for i, sc in enumerate(scenarios):
        where = f"scenario[{i}]"
        if not isinstance(sc, dict):
            errors.append(f"{where}: not a mapping")
            continue
        sid = sc.get("id")
        if not isinstance(sid, str) or not sid:
            errors.append(f"{where}: missing id")
            continue
        where = sid
        if sid in seen_ids:
            errors.append(f"{where}: duplicate id")
        seen_ids.add(sid)

        for key in ("ask", "expect", "questions", "artifacts", "why"):
            if key not in sc:
                errors.append(f"{where}: missing {key}")
        if not isinstance(sc.get("ask"), str) or not sc["ask"].strip():
            errors.append(f"{where}: ask is empty")

        expect = sc.get("expect")
        if not isinstance(expect, dict):
            errors.append(f"{where}: expect is not a mapping")
            expect = {}
        unknown = set(expect) - EXPECT_KEYS
        if unknown:
            errors.append(f"{where}: unknown expect keys: {', '.join(sorted(unknown))}")
        for key in ("engine", "runtime", "level", "capability", "record"):
            if key not in expect:
                errors.append(f"{where}: expect.{key} is missing")
        if expect.get("engine") not in engines:
            errors.append(f"{where}: expect.engine {expect.get('engine')!r} is not in engines.json")
        else:
            seen_engines.add(expect["engine"])
        if expect.get("runtime") not in RUNTIMES:
            errors.append(f"{where}: expect.runtime must be shared|dedicated")
        if norm(expect.get("level")) not in LEVELS:
            errors.append(f"{where}: expect.level must be 0|1|2|3")
        if expect.get("capability") not in CAPABILITIES:
            errors.append(f"{where}: expect.capability must be one of {sorted(CAPABILITIES)}")
        if "library" in expect and expect["library"] not in LIBRARIES:
            errors.append(f"{where}: expect.library must be one of {sorted(LIBRARIES)}")
        record = expect.get("record")
        if not isinstance(record, str) or not record.strip():
            errors.append(f"{where}: expect.record is empty")

        questions = sc.get("questions")
        if not isinstance(questions, dict):
            errors.append(f"{where}: questions is not a mapping")
            questions = {}
        limit = questions.get("max")
        if not isinstance(limit, int) or isinstance(limit, bool) or not 0 <= limit <= 3:
            errors.append(f"{where}: questions.max must be an integer 0..3")
        forbid = questions.get("forbid", [])
        if not isinstance(forbid, list) or not all(isinstance(p, str) and p for p in forbid):
            errors.append(f"{where}: questions.forbid must be a list of non-empty strings")

        artifacts = sc.get("artifacts")
        if not isinstance(artifacts, list) or not artifacts:
            errors.append(f"{where}: artifacts must be a non-empty list")
        else:
            for rel in artifacts:
                if not isinstance(rel, str) or not (ROOT / rel).exists():
                    errors.append(f"{where}: artifact missing: {rel}")

        if not isinstance(sc.get("why"), str) or not sc["why"].strip():
            errors.append(f"{where}: why is empty")

    missing = engines - seen_engines
    if missing:
        errors.append("no scenario covers engine(s): " + ", ".join(sorted(missing)))
    if not any(
        isinstance(sc.get("questions"), dict) and sc["questions"].get("max") == 0
        for sc in scenarios
    ):
        errors.append("no scenario has questions.max: 0 (the over-asking guard is untested)")
    return errors


def grade(scenario: dict, transcript: dict) -> list[str]:
    errors: list[str] = []
    sid = scenario["id"]
    if not isinstance(transcript, dict):
        return [f"{sid}: transcript is not a JSON object"]
    if transcript.get("scenario") != sid:
        errors.append(f"{sid}: transcript.scenario is {transcript.get('scenario')!r}")

    questions = transcript.get("questions")
    if not isinstance(questions, list):
        errors.append(f"{sid}: questions must be a list")
        questions = []
    limit = scenario["questions"]["max"]
    if len(questions) > limit:
        errors.append(f"{sid}: asked {len(questions)} question(s), limit is {limit}")
    forbid = scenario["questions"].get("forbid", [])
    for i, q in enumerate(questions):
        where = f"{sid}: question[{i}]"
        if not isinstance(q, dict):
            errors.append(f"{where}: not an object")
            continue
        text = q.get("text")
        if not isinstance(text, str) or not text.strip():
            errors.append(f"{where}: text is empty")
            text = ""
        if not isinstance(q.get("recommended"), str) or not q["recommended"].strip():
            errors.append(f"{where}: missing recommendation (recommended)")
        options = q.get("options")
        if not isinstance(options, list) or len(options) < 2:
            errors.append(f"{where}: options must be a closed list of at least 2")
        for pattern in forbid:
            if re.search(pattern, text, re.I):
                errors.append(f"{where}: re-asks a fact the scenario gives: /{pattern}/")

    decision = transcript.get("decision")
    if not isinstance(decision, dict):
        errors.append(f"{sid}: decision must be an object")
        decision = {}
    expect = scenario["expect"]
    for key in ("engine", "runtime", "level", "capability"):
        if key not in decision:
            errors.append(f"{sid}: decision missing {key}")
        elif norm(decision[key]) != norm(expect[key]):
            errors.append(f"{sid}: decision.{key}={decision[key]!r}, expected {expect[key]!r}")
    if "library" in expect and norm(decision.get("library")) != norm(expect["library"]):
        errors.append(f"{sid}: decision.library={decision.get('library')!r}, expected {expect['library']!r}")
    record = decision.get("record")
    if not isinstance(record, str) or not record.strip():
        errors.append(f"{sid}: decision.record is empty")
    elif norm(record) != norm(expect["record"]):
        errors.append(f"{sid}: decision.record={record!r}, expected {expect['record']!r}")
    return errors


def self_test(scenarios: list[dict]) -> tuple[list[str], int]:
    """Prove the grader rejects the failures it exists to catch."""
    errors: list[str] = []
    rejected = 0
    base: dict[str, dict] = {}

    def expect_pass(sc: dict) -> None:
        errs = grade(sc, base[sc["id"]])
        if errs:
            errors.append(f"self-test {sc['id']}: valid transcript rejected: {errs[0]}")

    def expect_fail(name: str, sc: dict, transcript: dict) -> None:
        nonlocal rejected
        if grade(sc, transcript):
            rejected += 1
        else:
            errors.append(f"self-test {name}: mutation was accepted")

    def copy(sc: dict) -> dict:
        return json.loads(json.dumps(base[sc["id"]]))

    for sc in scenarios:
        base[sc["id"]] = {
            "scenario": sc["id"],
            "questions": [],
            "decision": {k: str(v) for k, v in sc["expect"].items()},
        }
        expect_pass(sc)

    sc = scenarios[0]
    t = copy(sc)
    engines = {s["expect"]["engine"] for s in scenarios}
    t["decision"]["engine"] = sorted(engines - {sc["expect"]["engine"]})[0]
    expect_fail("wrong-engine", sc, t)

    sc = next(s for s in scenarios if s["questions"]["max"] > 0)
    t = copy(sc)
    t["questions"] = [
        {"text": f"question {i}", "recommended": "x", "options": ["a", "b"]}
        for i in range(sc["questions"]["max"] + 1)
    ]
    expect_fail("too-many-questions", sc, t)

    t = copy(sc)
    t["questions"] = [{"text": "only one?", "options": ["a", "b"]}]
    expect_fail("missing-recommendation", sc, t)

    sc = next(s for s in scenarios if s["questions"].get("forbid"))
    t = copy(sc)
    t["questions"] = [
        {
            "text": sc["questions"]["forbid"][0],
            "recommended": "x",
            "options": ["a", "b"],
        }
    ]
    expect_fail("forbidden-question", sc, t)

    sc = scenarios[0]
    t = copy(sc)
    del t["decision"]["record"]
    expect_fail("missing-record", sc, t)

    t = copy(sc)
    del t["decision"]["engine"]
    expect_fail("missing-branch", sc, t)

    return errors, rejected


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--check", action="store_true", help="validate the scenario fixtures")
    ap.add_argument("--self-test", action="store_true", help="prove the grader rejects mutations")
    ap.add_argument("--grade", metavar="TRANSCRIPT", help="grade one transcript JSON")
    args = ap.parse_args(argv)
    if not (args.check or args.self_test or args.grade):
        ap.error("choose --check, --self-test or --grade")

    try:
        scenarios = load_scenarios()
    except Exception as exc:  # noqa: BLE001 - one clear CLI error
        print(f"decision-eval: {exc}", file=sys.stderr)
        return 1

    errors: list[str] = []
    if args.check:
        errors.extend(check_scenarios(scenarios, load_engines()))
    if args.self_test:
        errs, rejected = self_test(scenarios)
        errors.extend(errs)
        if not errs:
            print(f"decision-eval: self-test ok ({rejected} mutations rejected)")
    if args.grade:
        by_id = {s["id"]: s for s in scenarios}
        try:
            transcript = json.loads(Path(args.grade).read_text())
        except Exception as exc:  # noqa: BLE001 - one clear CLI error
            print(f"decision-eval: {args.grade}: {exc}", file=sys.stderr)
            return 1
        sid = transcript.get("scenario") if isinstance(transcript, dict) else None
        if sid not in by_id:
            errors.append(f"{args.grade}: unknown scenario {sid!r}")
        else:
            errors.extend(f"{args.grade}: {e}" for e in grade(by_id[sid], transcript))

    if errors:
        for err in errors:
            print(f"decision-eval: FAIL: {err}", file=sys.stderr)
        return 1
    if args.check:
        print(f"decision-eval: ok ({len(scenarios)} scenarios, {len(load_engines())} engine(s))")
    return 0


if __name__ == "__main__":
    sys.exit(main())
