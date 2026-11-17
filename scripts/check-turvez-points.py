#!/usr/bin/env python3
"""Check that every branch in `cmd/turvez` still points at something true.

The router claims "this ask → this doc → this section → this gate". A doc can be
reorganised, and then the router is confidently pointing at a heading somebody
deleted. That is exactly the failure a manual doc list has, so it is checked
here instead of trusted.

The shell half of `TestTurvezPoints` in `cmd/turvez/main_test.go`: this runs
without a Go toolchain, so `scripts/test-decisions.sh` catches a moved heading on
a machine that only cloned the repo to read it.
"""

import pathlib
import re
import sys

# Parsed per line and by position rather than one wide regex: each branch is a
# single-line struct literal, so the quoted strings are, in order, the id, the
# ask, the answer, the doc, the gate, the trigger words and the section.
QUOTED = re.compile(r'"((?:[^"\\]|\\.)*)"')


def branch_rows(src: str):
    for line in src.splitlines():
        if '{"Q' not in line:
            continue
        parts = QUOTED.findall(line)
        docs = [i for i, p in enumerate(parts) if p.startswith("docs/")]
        if len(parts) < 4 or not docs:
            continue
        i = docs[0]
        yield parts[0], parts[i].split("#")[0], parts[i + 1], parts[-1]


def plain(text: str) -> str:
    """Heading text without its decoration, so restyling is not a false alarm."""
    for deco in ("**", "`", "\u2014", "\u2013"):
        text = text.replace(deco, "-" if deco in "\u2014\u2013" else "")
    return " ".join(text.lower().split())


def main() -> int:
    root = pathlib.Path(__file__).resolve().parent.parent
    main_go = root / "cmd" / "turvez" / "main.go"
    if not main_go.exists():
        print("check-turvez-points: cmd/turvez/main.go is missing", file=sys.stderr)
        return 1

    bad = 0
    seen = 0
    for qid, doc, gate, section in branch_rows(main_go.read_text()):
        seen += 1
        path = root / doc
        if not path.exists():
            print(f"FAIL: {qid} points at {doc}, which is not in the repo", file=sys.stderr)
            bad += 1
            continue
        if not gate.strip():
            print(f"FAIL: {qid} has no gate", file=sys.stderr)
            bad += 1
        headings = {plain(line[3:]) for line in path.read_text().splitlines() if line.startswith("## ")}
        if plain(section) not in headings:
            print(
                f"FAIL: {qid} points at {doc} section {section!r}, which is not a "
                f"heading there (the doc moved and the router believes the old shape)",
                file=sys.stderr,
            )
            bad += 1

    if seen == 0:
        print("check-turvez-points: no branch rows found in cmd/turvez/main.go", file=sys.stderr)
        return 1
    if bad:
        return 1
    print(f"check-turvez-points: ok ({seen} branch(es))")
    return 0


if __name__ == "__main__":
    sys.exit(main())
