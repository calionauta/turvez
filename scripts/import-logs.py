#!/usr/bin/env python3
"""Import unlabeled inputs from logs into a spreadsheet-ready CSV.

When a client has only logs there is no production output to suggest a label.
This turns log lines into `cases/logs.csv`, one row per unique input, with an
empty `expected_category` for a human to fill. It never invents a label.

    python3 scripts/import-logs.py --input logs.jsonl --text-field message \
        --out evals/acme

Reads JSONL, a JSON array, or CSV. Writes `cases/logs.csv` with columns
`text, expected_category, __metadata:source, __metadata:split`.
Use it with the logs config template (`promptfooconfig.logs.yaml`).
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import sys
from pathlib import Path


TEXT_CANDIDATES = ("text", "message", "msg", "body", "content", "log", "line")


def pick(row: dict, field: str) -> str:
    """Use the requested field, else the first common log field present."""
    if field in row:
        return str(row[field])
    for candidate in TEXT_CANDIDATES:
        if candidate in row:
            return str(row[candidate])
    return ""


def rows(path: Path, text_field: str, id_field: str | None):
    if path.suffix in (".txt", ".log"):
        for number, line in enumerate(path.read_text().splitlines(), 1):
            if line.strip():
                yield line, str(number)
        return
    if path.suffix == ".csv":
        with path.open(newline="") as fh:
            reader = csv.DictReader(fh)
            for number, row in enumerate(reader, 1):
                yield pick(row, text_field), row.get(id_field or "", str(number))
        return
    if path.suffix in (".jsonl", ".ndjson"):
        for number, line in enumerate(path.read_text().splitlines(), 1):
            if line.strip():
                doc = json.loads(line)
                yield pick(doc, text_field), str(doc.get(id_field or "", number))
        return
    doc = json.loads(path.read_text())
    items = doc.get("items", doc) if isinstance(doc, dict) else doc
    for number, item in enumerate(items, 1):
        yield pick(item, text_field), str(item.get(id_field or "", number))


def main() -> int:
    ap = argparse.ArgumentParser(
        description="Turn logs into an unlabeled CSV for human labeling.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""examples:
  python3 scripts/import-logs.py --input logs.jsonl --text-field message --out evals/acme
  python3 scripts/import-logs.py --input logs.csv --text-field body --out evals/acme --max 200

no label is generated. fill expected_category in the CSV, then run
promptfooconfig.logs.yaml. see skills/turvez/references/eval-loop.md.""")
    ap.add_argument("--input", required=True, help="logs file (.jsonl, .json, .csv, .txt, .log)")
    ap.add_argument("--text-field", default="text", help="field/column with the text; falls back to message/msg/body/content/log")
    ap.add_argument("--id-field", default=None, help="optional id field for traceability")
    ap.add_argument("--out", required=True, help="client eval dir (writes cases/logs.csv)")
    ap.add_argument("--split", default="dev", help="metadata split (dev|holdout)")
    ap.add_argument("--max", type=int, default=200, help="cap the number of rows")
    args = ap.parse_args()

    seen: set[str] = set()
    out_rows: list[dict] = []
    for text, source_id in rows(Path(args.input), args.text_field, args.id_field):
        text = text.strip()
        if not text:
            continue
        digest = hashlib.sha256(text.encode()).hexdigest()[:16]
        if digest in seen:
            continue
        seen.add(digest)
        out_rows.append({
            "text": text,
            "expected_category": "",
            "__metadata:source": f"logs:{source_id}" if source_id else "logs",
            "__metadata:split": args.split,
        })
        if len(out_rows) >= args.max:
            break

    if not out_rows:
        print("no usable rows found")
        return 1

    target = Path(args.out) / "cases" / "logs.csv"
    target.parent.mkdir(parents=True, exist_ok=True)
    with target.open("w", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=list(out_rows[0]))
        writer.writeheader()
        writer.writerows(out_rows)

    print(f"imported {len(out_rows)} unique row(s) -> {target}")
    print("next: fill expected_category, then run promptfooconfig.logs.yaml")
    return 0


if __name__ == "__main__":
    sys.exit(main())
