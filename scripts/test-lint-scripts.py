#!/usr/bin/env python3
"""Test suite for scripts/lint-scripts.py.

The gate has two jobs, and both are pinned here:
- the language policy (Go nodes; TypeScript/Python only for a WAC orchestrator)
- the static checks that `wmill lint` cannot do (undefined names, compile
  errors, and flow <-> script signature mismatches)
"""

import os
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
LINT_PY = ROOT / "scripts" / "lint-scripts.py"

GO_HELLO = """package inner

func main() (interface{}, error) {
	return map[string]any{"ok": true}, nil
}
"""

WAC_PY = '''from wmill import workflow, task_script

extract = task_script("f/acme/extract")


@workflow
async def main(limit: int):
    data = await extract(limit=limit)
    return {"count": len(data)}
'''

WAC_TS = '''import { taskScript, workflow } from "windmill-client";

const extract = taskScript("f/acme/extract");

export const main = workflow(async (limit: number) => {
  const data = await extract({ limit });
  return { count: data.length };
});
'''


def run_linter(path, env=None):
    cmd = [sys.executable, str(LINT_PY), str(path)]
    return subprocess.run(cmd, capture_output=True, text=True, env=env)


def flow(modules):
    lines = ["summary: test flow", "value:", "  modules:"]
    for mod in modules:
        lines.append("    - id: %s" % mod["id"])
        lines.append("      value:")
        lines.append("        type: rawscript")
        lines.append("        language: %s" % mod["language"])
        lines.append("        input_transforms: %s" % mod.get("transforms", "{}"))
        lines.append("        content: '!inline %s'" % mod["file"])
    return "\n".join(lines) + "\n"


def tmp_flow(tmp, modules, files):
    flow_dir = Path(tmp) / "flow"
    flow_dir.mkdir()
    (flow_dir / "flow.yaml").write_text(flow(modules))
    for name, body in files.items():
        (flow_dir / name).write_text(body)
    return flow_dir


def assert_fails(res, *needles):
    assert res.returncode != 0, "expected failure, got:\n%s\n%s" % (
        res.stdout,
        res.stderr,
    )
    combined = res.stdout + res.stderr
    for needle in needles:
        assert needle in combined, "expected %r in:\n%s" % (needle, combined)


def assert_passes(res):
    assert res.returncode == 0, "expected pass, got:\n%s\n%s" % (res.stdout, res.stderr)


# --------------------------------------------------------------------------- #
# policy: Go nodes, TS/Python only for the graph
# --------------------------------------------------------------------------- #


def test_clean_go_flow():
    with tempfile.TemporaryDirectory() as tmp:
        d = tmp_flow(
            tmp,
            [{"id": "step_one", "language": "go", "file": "step_one.go"}],
            {"step_one.go": GO_HELLO},
        )
        assert_passes(run_linter(d))
        print("ok: clean Go node passes")


def test_python_node_rejected():
    with tempfile.TemporaryDirectory() as tmp:
        d = tmp_flow(
            tmp,
            [{"id": "step_one", "language": "python3", "file": "step_one.py"}],
            {"step_one.py": "def main():\n    return 1\n"},
        )
        assert_fails(run_linter(d), "must be Go", "language-policy")
        print("ok: Python logic node rejected with a policy message")


def test_ts_node_rejected():
    with tempfile.TemporaryDirectory() as tmp:
        d = tmp_flow(
            tmp,
            [{"id": "step_one", "language": "deno", "file": "step_one.ts"}],
            {"step_one.ts": "export async function main() {\n  return 1;\n}\n"},
        )
        assert_fails(run_linter(d), "must be Go")
        print("ok: TypeScript logic node rejected")


def test_stray_python_rejected():
    """A .py sitting in a windmill dir is automation code even if no flow uses it."""
    with tempfile.TemporaryDirectory() as tmp:
        d = tmp_flow(
            tmp,
            [{"id": "step_one", "language": "go", "file": "step_one.go"}],
            {"step_one.go": GO_HELLO, "helper.py": "def f():\n    return 1\n"},
        )
        assert_fails(run_linter(d), "helper.py", "must be Go")
        print("ok: stray Python file rejected")


def test_wac_python_orchestrator_allowed():
    with tempfile.TemporaryDirectory() as tmp:
        d = Path(tmp) / "windmill"
        d.mkdir()
        (d / "orchestrator.py").write_text(WAC_PY)
        res = run_linter(d)
        assert_passes(res)
        assert "WAC orchestrator" in res.stdout, res.stdout
        print("ok: WAC Python orchestrator allowed")


def test_wac_ts_orchestrator_allowed():
    with tempfile.TemporaryDirectory() as tmp:
        d = Path(tmp) / "windmill"
        d.mkdir()
        (d / "orchestrator.ts").write_text(WAC_TS)
        assert_passes(run_linter(d))
        print("ok: WAC TypeScript orchestrator allowed")


# --------------------------------------------------------------------------- #
# Go static checks
# --------------------------------------------------------------------------- #


def test_go_syntax_error():
    with tempfile.TemporaryDirectory() as tmp:
        broken = Path(tmp) / "broken.go"
        broken.write_text("package inner\n\nfunc main() (interface{}, error) {\n")
        assert_fails(run_linter(broken), "broken.go")
        print("ok: Go syntax error caught")


def test_go_missing_main():
    with tempfile.TemporaryDirectory() as tmp:
        f = Path(tmp) / "helper.go"
        f.write_text("package inner\n\nfunc helper() {}\n")
        assert_fails(run_linter(f), "no `func main")
        print("ok: missing main caught")


def test_go_missing_error_return():
    with tempfile.TemporaryDirectory() as tmp:
        f = Path(tmp) / "noerr.go"
        f.write_text("package inner\n\nfunc main() int {\n\treturn 1\n}\n")
        assert_fails(run_linter(f), "must return (interface{}, error)")
        print("ok: non-erroring main caught")


def test_go_type_error():
    with tempfile.TemporaryDirectory() as tmp:
        f = Path(tmp) / "typeerr.go"
        f.write_text(
            'package inner\n\nfunc main() (interface{}, error) {\n\tvar s string = 42\n\treturn s, nil\n}\n'
        )
        assert_fails(run_linter(f), "does not compile")
        print("ok: Go type error caught")


def test_go_binding_missing_param():
    with tempfile.TemporaryDirectory() as tmp:
        d = tmp_flow(
            tmp,
            [{"id": "s1", "language": "go", "file": "s1.go"}],
            {
                "s1.go": 'package inner\n\nfunc main(req_param string) (interface{}, error) {\n\treturn req_param, nil\n}\n'
            },
        )
        assert_fails(run_linter(d), "takes 'req_param'")
        print("ok: missing input_transforms entry caught")


def test_go_binding_extra_transform():
    with tempfile.TemporaryDirectory() as tmp:
        d = tmp_flow(
            tmp,
            [
                {
                    "id": "s1",
                    "language": "go",
                    "file": "s1.go",
                    "transforms": "{unexpected_arg: {type: javascript, expr: '1'}}",
                }
            ],
            {"s1.go": GO_HELLO},
        )
        assert_fails(run_linter(d), "provides 'unexpected_arg'")
        print("ok: unexpected input_transforms entry caught")


def test_missing_inline_file():
    with tempfile.TemporaryDirectory() as tmp:
        d = tmp_flow(tmp, [{"id": "s1", "language": "go", "file": "ghost.go"}], {})
        assert_fails(run_linter(d), "inline script not found")
        print("ok: missing inline script caught")


# --------------------------------------------------------------------------- #
# Python static checks (WAC orchestrators only)
# --------------------------------------------------------------------------- #


def test_wac_undefined_name():
    with tempfile.TemporaryDirectory() as tmp:
        f = Path(tmp) / "orchestrator.py"
        f.write_text(
            "from wmill import workflow, task_script\n\n"
            "extract = task_script('f/acme/extract')\n\n\n"
            "@workflow\nasync def main():\n    return UNDEFINED_VAR\n"
        )
        assert_fails(run_linter(f), "UNDEFINED_VAR")
        print("ok: undefined name in a WAC orchestrator caught")


def test_wac_ast_fallback_mode():
    """AST fallback when ruff is not in PATH (the graph still gets checked)."""
    env = dict(os.environ, PATH="/usr/bin:/bin")
    with tempfile.TemporaryDirectory() as tmp:
        f = Path(tmp) / "orchestrator.py"
        f.write_text(
            "from wmill import workflow, task_script\n\n"
            "extract = task_script('f/acme/extract')\n\n\n"
            "@workflow\nasync def main():\n    return UNDEFINED_VAR\n"
        )
        assert_fails(run_linter(f, env=env), "UNDEFINED_VAR")
        print("ok: AST fallback works without ruff in PATH")


def main():
    test_clean_go_flow()
    test_python_node_rejected()
    test_ts_node_rejected()
    test_stray_python_rejected()
    test_wac_python_orchestrator_allowed()
    test_wac_ts_orchestrator_allowed()
    test_go_syntax_error()
    test_go_missing_main()
    test_go_missing_error_return()
    test_go_type_error()
    test_go_binding_missing_param()
    test_go_binding_extra_transform()
    test_missing_inline_file()
    test_wac_undefined_name()
    test_wac_ast_fallback_mode()
    print("ALL TESTS PASSED!")


if __name__ == "__main__":
    main()
