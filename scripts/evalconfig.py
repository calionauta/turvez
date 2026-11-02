"""Shared eval config for the turvez scripts.

One file per client, `clients/<client>/evals/eval.config.json`, describes what to pull
from run history and what the operator should label. The scripts fall back to
the support-ticket example when the file is absent, so the template keeps
working out of the box.

Nothing here is client-specific by construction: steps, label fields, values,
shortcuts and the rubric all come from the config.

For retrieval evals, the same shape is used, but the labeled fields and rubric
describe answer quality, citation validity and refusal correctness rather than a
single-step triage output. The retrieval playbook is `skills/turvez/references/retrieval.md`;
the pipeline posture is `docs/retrieval.md`.
"""

from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path

DEFAULTS: dict = {
    "prompt_step": "build-prompt",
    "result_step": "triage",
    "gate_step": "gate",
    "text_var": "text",
    "text_role": "user",
    "fields": [
        {
            "name": "category",
            "var": "expected_category",
            "values": ["billing", "technical", "other"],
            "keys": {"b": "billing", "t": "technical", "o": "other"},
        }
    ],
    # Extra JSON-schema properties that are validated but not labeled.
    "schema_extra": {"urgency": ["low", "high"]},
    "rubric": (
        "Ticket: {{text}}\n\n"
        "The JSON fields must match the ticket's actual intent and be defensible."
    ),
    # Retrieval evals reuse this config shape. The fields below are examples for
    # an answer/citation/refusal eval, not the only taxonomy.
    "retrieval": False,
    "retrieval_answer_field": "expected_answer_summary",
    "retrieval_answer_values": ["covers", "partial", "wrong", "not_in_corpus"],
    "retrieval_citation_field": "expected_citation_valid",
    "retrieval_citation_values": ["valid", " hallucinated", " out_of_selected_set"],
    "retrieval_refusal_field": "expected_refusal",
    "retrieval_refusal_values": ["refused", "answered"],
}


def load(path: Path | None) -> dict:
    """Load a client config over the defaults. Missing keys keep defaults."""
    cfg = deepcopy(DEFAULTS)
    if path is not None and path.exists():
        user = json.loads(path.read_text())
        for key, value in user.items():
            if value is not None:
                cfg[key] = value
    return cfg


def default_path(eval_dir: Path) -> Path:
    return eval_dir / "eval.config.json"


def schema(cfg: dict) -> dict:
    """JSON schema for the is-json assertion, built from the config."""
    properties: dict = {}
    required: list[str] = []
    for field in cfg.get("fields") or []:
        properties[field["name"]] = {"enum": list(field["values"])}
        required.append(field["name"])
    for name, values in (cfg.get("schema_extra") or {}).items():
        if name in properties:
            continue  # a labeled field already owns this property
        properties[name] = {"enum": list(values)}
        required.append(name)
    return {"type": "object", "required": required, "properties": properties}


def fields(cfg: dict) -> list[dict]:
    return cfg.get("fields") or []


def matches_schema(value, cfg: dict) -> bool:
    """True when a parsed model output carries every configured field."""
    if not isinstance(value, dict):
        return False
    for field in fields(cfg):
        if value.get(field["name"]) not in field["values"]:
            return False
    for name, values in (cfg.get("schema_extra") or {}).items():
        if name in {f["name"] for f in fields(cfg)}:
            continue
        if value.get(name) not in values:
            return False
    return True


def retrieval_fields(cfg: dict) -> list[dict]:
    """Labeling fields for a retrieval eval: answer, citation, refusal.

    This is a starter taxonomy. Clients may replace it, add a route label, or
    add a per-query-type label without touching the scripts.
    """
    if not cfg.get("retrieval"):
        return []
    base: list[dict] = []
    for name, var, values, keys in [
        ("answer_summary",
         cfg.get("retrieval_answer_field", "expected_answer_summary"),
         cfg.get("retrieval_answer_values", ["covers", "partial", "wrong", "not_in_corpus"]),
         None),
        ("citation_valid",
         cfg.get("retrieval_citation_field", "expected_citation_valid"),
         cfg.get("retrieval_citation_values", ["valid", " hallucinated", " out_of_selected_set"]),
         None),
        ("refusal",
         cfg.get("retrieval_refusal_field", "expected_refusal"),
         cfg.get("retrieval_refusal_values", ["refused", "answered"]),
         None),
    ]:
        base.append({"name": name, "var": var, "values": values, "keys": keys})
    return base


def fields(cfg: dict) -> list[dict]:
    return cfg.get("fields") or []


def retrieval_enabled(cfg: dict) -> bool:
    return bool(cfg.get("retrieval"))


def _unique_prefix(value: str, values: list[str]) -> str:
    """Shortest prefix that no other value starts with.

    "high"/"hold" -> "hi"/"ho"; "low"/"lower" -> "low"/"lowe".
    """
    lower = value.lower()
    others = [v.lower() for v in values if v != value]
    for n in range(1, len(lower) + 1):
        prefix = lower[:n]
        if not any(other.startswith(prefix) for other in others):
            return prefix
    return lower


def shortcuts(field: dict) -> dict[str, str]:
    """Explicit keys from the config, else shortest unique prefixes.

    Keys are case-insensitive. If two computed keys collide, append a digit.
    """
    explicit = field.get("keys") or {}
    if explicit:
        return {str(k).lower(): v for k, v in explicit.items()}

    values = list(dict.fromkeys(field["values"]))  # preserve order, dedupe
    out: dict[str, str] = {}
    for value in values:
        key = _unique_prefix(value, values)
        while key in out:
            key += str(len(out) + 1)
        out[key] = value
    return out
