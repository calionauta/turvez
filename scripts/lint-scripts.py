#!/usr/bin/env python3
"""Local gate for Windmill automation scripts and their flow bindings.

Addresses the bottleneck mapped in docs/spike-local-script-gate.md: `wmill lint`
validates the flow schema, not the script body. A script with a syntax error, an
undefined name or a signature that does not match its `input_transforms` lints
clean and only fails after push, at run time.

What this gate enforces
-----------------------
1. **Language policy** (docs/language-policy.md). Every script *node* is Go, in
   both engines. TypeScript and Python are allowed only for a worklow-as-code
   *orchestrator* — a file that calls `workflow()` and does nothing but sequence
   Go scripts through `taskScript()` / `taskFlow()`. Windmill only supports
   workflows-as-code in TypeScript and Python, so the graph is the one place
   where those languages are legitimate.
2. **Go static checks** on every `.go` under a Windmill directory:
   - `gofmt -e` (syntax errors + formatting)
   - compile in a throwaway module (type errors)
   - `func main(...)` exists and returns an error
   - `func main` parameters match the step's `input_transforms`
3. **Python static checks** on WAC orchestrators only: `ruff check
   --select F821,F401,F811,E9` with Windmill globals as builtins, falling back
   to a built-in AST walk when ruff is absent.
4. **Flow-to-script binding contract** for every step: `!inline <file>` exists,
   the step language matches the file, required parameters are provided, and no
   transform passes a parameter the script does not accept.

Usage:
    ./scripts/lint-scripts.sh
    python3 scripts/lint-scripts.py [PATH...]
    TURVEZ_OPS_DIR=../turvez-ops ./scripts/lint-scripts.sh
"""

import argparse
import ast
import builtins
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent

# Languages allowed for a script node. Everything else is either the WAC
# orchestrator exception or a policy violation.
NODE_LANGUAGE = "go"
GRAPH_LANGUAGES = {"python3", "deno", "bun"}
FLOW_TYPES = {"rawscript"}

try:
    import yaml

    class FlowLoader(yaml.SafeLoader):
        pass

    FlowLoader.add_constructor(
        "!inline", lambda loader, node: loader.construct_scalar(node)
    )
except ImportError:
    yaml = None


# --------------------------------------------------------------------------- #
# discovery
# --------------------------------------------------------------------------- #


def find_windmill_dirs(search_roots):
    wm_dirs = []
    for r in search_roots:
        if not r.is_dir():
            continue
        for p in r.rglob("windmill"):
            if p.is_dir():
                wm_dirs.append(p)
    return sorted(set(wm_dirs))


def find_flow_yamls(wm_dirs):
    flows = []
    for d in wm_dirs:
        for p in d.rglob("flow.yaml"):
            flows.append(p)
    return sorted(flows)


def _skip(path):
    return any(part.startswith(".") or part == "__pycache__" for part in path.parts)


def find_by_suffix(wm_dirs, *suffixes):
    files = []
    for d in wm_dirs:
        for p in d.rglob("*"):
            if p.is_file() and not _skip(p) and p.suffix in suffixes:
                files.append(p)
    return sorted(files)


# --------------------------------------------------------------------------- #
# policy: Go nodes, TS/Python only for the graph
# --------------------------------------------------------------------------- #


def wac_orchestrator_reason(path):
    """Return why `path` qualifies as a WAC orchestrator, or None.

    Windmill supports workflows-as-code in TypeScript and Python only
    (docs: core_concepts/workflows_as_code). The orchestrator is allowed there
    because the product leaves no alternative — but it may not contain business
    logic: it must import the workflow primitives and dispatch to existing
    scripts/flows with taskScript/taskFlow.
    """
    try:
        src = path.read_text(encoding="utf-8")
    except OSError:
        return None

    code = "\n".join(
        line for line in src.splitlines() if not line.lstrip().startswith("//")
    )

    if path.suffix == ".py":
        imports_workflow = bool(
            re.search(r"from\s+wmill\s+import[^\n]*\bworkflow\b", code)
            or re.search(r"import\s+wmill\b", code)
        )
        dispatches = bool(re.search(r"\btask_script\s*\(", code)) or bool(
            re.search(r"\btask_flow\s*\(", code)
        )
        entrypoint = bool(re.search(r"@workflow\b|workflow\s*\(\s*\)", code))
    elif path.suffix in (".ts", ".js", ".mjs"):
        imports_workflow = bool(
            re.search(r"from\s+['\"]windmill-client['\"]", code)
        )
        dispatches = bool(re.search(r"\btaskScript\s*\(", code)) or bool(
            re.search(r"\btaskFlow\s*\(", code)
        )
        entrypoint = bool(re.search(r"\bworkflow\s*\(", code))
    else:
        return None

    if imports_workflow and dispatches and entrypoint:
        return "workflow-as-code orchestrator"
    return None


def check_language_policy(wm_dirs):
    """Every non-Go source file must be a WAC orchestrator. Go is the node."""
    errors = []
    graph = []
    for path in find_by_suffix(wm_dirs, ".py", ".ts", ".js", ".mjs"):
        reason = wac_orchestrator_reason(path)
        if reason:
            graph.append(path)
            continue
        errors.append(
            "%s: %s scripts must be Go (docs/language-policy.md).\n"
            "      Allowed non-Go file: a workflow-as-code orchestrator that calls\n"
            "      workflow() and dispatches with taskScript()/taskFlow(). This one\n"
            "      has logic, so move the logic into a Go node."
            % (path, path.suffix)
        )
    return errors, graph


# --------------------------------------------------------------------------- #
# Go checks
# --------------------------------------------------------------------------- #


def _strip_strings_and_comments(src):
    """Blank out string/char literals and comments, keeping offsets stable."""
    out = []
    i = 0
    n = len(src)
    while i < n:
        ch = src[i]
        if ch == "/" and i + 1 < n and src[i + 1] == "/":
            while i < n and src[i] != "\n":
                out.append(" ")
                i += 1
        elif ch == "/" and i + 1 < n and src[i + 1] == "*":
            while i < n and not (src[i] == "*" and i + 1 < n and src[i + 1] == "/"):
                out.append("\n" if src[i] == "\n" else " ")
                i += 1
            out.append("  ")
            i += 2
        elif ch in ('"', "'", "`"):
            quote = ch
            out.append(" ")
            i += 1
            while i < n:
                if src[i] == "\\" and quote != "`":
                    out.append(" ")
                    i += 1
                    if i < n:
                        out.append(" ")
                        i += 1
                    continue
                if src[i] == quote:
                    out.append(" ")
                    i += 1
                    break
                out.append("\n" if src[i] == "\n" else " ")
                i += 1
        else:
            out.append(ch)
            i += 1
    return "".join(out)


def parse_go_main(source):
    """Return (params, returns_error, exists) for `func main` in a Go script.

    Windmill's Go contract: a `main` function whose parameters become the input
    schema and which returns `(interface{}, error)`.
    """
    code = _strip_strings_and_comments(source)
    match = re.search(r"\bfunc\s+main\s*\(", code)
    if not match:
        return [], False, False

    start = match.end() - 1
    depth = 0
    end = None
    for i in range(start, len(code)):
        if code[i] == "(":
            depth += 1
        elif code[i] == ")":
            depth -= 1
            if depth == 0:
                end = i
                break
    if end is None:
        return [], False, True

    param_src = code[start + 1 : end]
    params = _split_go_params(param_src)

    # Return clause: the rest of the line after the parameter list. gofmt keeps
    # it on one line, which avoids mistaking the braces of `interface{}` for the
    # function body.
    tail = code[end + 1 :]
    newline = tail.find("\n")
    returns = tail if newline < 0 else tail[:newline]
    return params, "error" in returns, True


def _split_go_params(param_src):
    """Split a Go parameter list, honouring grouped names and nested types."""
    parts = []
    cur = []
    depth = 0
    for ch in param_src:
        if ch in "([{":
            depth += 1
        elif ch in ")]}":
            depth -= 1
        if ch == "," and depth == 0:
            parts.append("".join(cur))
            cur = []
            continue
        cur.append(ch)
    if cur:
        parts.append("".join(cur))

    names = []
    previous_named = False
    for part in parts:
        part = " ".join(part.split())
        if not part:
            continue
        name, _, rest = part.partition(" ")
        if not rest.strip():
            # `func main(a, b string)` — a bare identifier continues the
            # previous parameter's type.
            if previous_named:
                names.append(name)
            continue
        names.append(name)
        previous_named = True
    return names


def gofmt_check(path):
    """Return (errors, parse_failed). gofmt names the file itself."""
    gofmt = shutil.which("gofmt")
    if not gofmt:
        return ["gofmt is required to lint Go scripts (install the Go toolchain)"], False
    res = subprocess.run(
        [gofmt, "-l", "-e", str(path)], capture_output=True, text=True
    )
    if res.returncode != 0:
        return [(res.stderr or res.stdout).strip() or "gofmt failed"], True
    if res.stdout.strip():
        return [f"{path}: not gofmt'd (run: gofmt -w {path})"], False
    return [], False


DEPENDENCY_ERRORS = (
    "no required module provides package",
    "cannot find module providing package",
    "missing go.sum entry",
    "unrecognized import path",
    "dial tcp",
    "no such host",
    "module lookup disabled",
)


def go_compile_check(path):
    """Compile one Go script in a throwaway module. Network-less by default."""
    go = shutil.which("go")
    if not go:
        return [], ["go is required to compile Go scripts"]

    with tempfile.TemporaryDirectory(prefix="wmlint-go-") as tmp:
        tmpdir = Path(tmp)
        (tmpdir / "go.mod").write_text("module wmlint\n\ngo 1.21\n", encoding="utf-8")
        shutil.copyfile(path, tmpdir / "script.go")
        env = dict(os.environ, GOWORK="off", GO111MODULE="on", GOFLAGS="-mod=mod")
        res = subprocess.run(
            [go, "build", "./..."],
            cwd=tmpdir,
            capture_output=True,
            text=True,
            env=env,
        )

    if res.returncode == 0:
        return [], []

    output = (res.stderr or res.stdout).strip()
    if any(marker in output for marker in DEPENDENCY_ERRORS):
        # A script that imports a third-party package cannot be compiled
        # offline. Say so instead of pretending it passed.
        return [], [f"{path}: compile skipped, dependency not resolvable offline"]
    return [f"{path}: does not compile:\n{_indent(output)}"], []


def _indent(text):
    return "\n".join("      " + line for line in text.splitlines())


def check_go_script(path):
    errors = []
    warnings = []

    try:
        source = path.read_text(encoding="utf-8")
    except OSError as exc:
        return [f"{path}: cannot read: {exc}"], []

    gofmt_errors, parse_failed = gofmt_check(path)
    errors.extend(gofmt_errors)

    if parse_failed:
        # A file that does not parse cannot be type-checked; one clear error
        # beats two that say the same thing.
        return errors, warnings

    params, returns_error, exists = parse_go_main(source)
    if not exists:
        errors.append(
            f"{path}: no `func main(...)` — Windmill scripts need one; its "
            "parameters become the input schema"
        )
    elif not returns_error:
        errors.append(
            "%s: `func main` must return (interface{}, error) — Windmill "
            "reads the error to fail the step" % path
        )

    compile_errors, compile_warnings = go_compile_check(path)
    errors.extend(compile_errors)
    warnings.extend(compile_warnings)
    return errors, warnings


# --------------------------------------------------------------------------- #
# Python checks (WAC orchestrators only)
# --------------------------------------------------------------------------- #


def ast_check_file(path):
    """Fallback AST checker when ruff is not installed."""
    errors = []
    try:
        content = path.read_text(encoding="utf-8")
        tree = ast.parse(content, filename=str(path))
    except SyntaxError as e:
        return [f"{path}:{e.lineno}:{e.offset}: SyntaxError: {e.msg}"]
    except Exception as e:
        return [f"{path}: failed to parse: {e}"]

    global_names = set(dir(builtins)) | {"wmill", "context"}

    for node in tree.body:
        if isinstance(node, ast.Import):
            for alias in node.names:
                global_names.add(alias.asname or alias.name.split(".")[0])
        elif isinstance(node, ast.ImportFrom):
            for alias in node.names:
                global_names.add(alias.asname or alias.name)
        elif isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
            global_names.add(node.name)
        elif isinstance(node, ast.Assign):
            for target in node.targets:
                for sub in ast.walk(target):
                    if isinstance(sub, ast.Name):
                        global_names.add(sub.id)
        elif isinstance(node, ast.AnnAssign):
            if isinstance(node.target, ast.Name):
                global_names.add(node.target.id)

    for node in tree.body:
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            fn_scope = set(global_names)
            for a in node.args.args + node.args.kwonlyargs:
                fn_scope.add(a.arg)
            if node.args.vararg:
                fn_scope.add(node.args.vararg.arg)
            if node.args.kwarg:
                fn_scope.add(node.args.kwarg.arg)

            for child in ast.walk(node):
                if isinstance(child, ast.Assign):
                    for target in child.targets:
                        for sub in ast.walk(target):
                            if isinstance(sub, ast.Name):
                                fn_scope.add(sub.id)
                elif isinstance(child, ast.AnnAssign):
                    if isinstance(child.target, ast.Name):
                        fn_scope.add(child.target.id)
                elif isinstance(child, (ast.For, ast.AsyncFor)):
                    for sub in ast.walk(child.target):
                        if isinstance(sub, ast.Name):
                            fn_scope.add(sub.id)
                elif isinstance(child, (ast.With, ast.AsyncWith)):
                    for item in child.items:
                        if item.optional_vars:
                            for sub in ast.walk(item.optional_vars):
                                if isinstance(sub, ast.Name):
                                    fn_scope.add(sub.id)
                elif isinstance(child, ast.ExceptHandler):
                    if child.name:
                        fn_scope.add(child.name)
                elif isinstance(child, ast.NamedExpr):
                    if isinstance(child.target, ast.Name):
                        fn_scope.add(child.target.id)

            for child in ast.walk(node):
                if isinstance(child, ast.Name) and isinstance(child.ctx, ast.Load):
                    if child.id not in fn_scope:
                        errors.append(
                            f"{path}:{child.lineno}:{child.col_offset}: Undefined name `{child.id}`"
                        )
    return errors


def ruff_check(files):
    cmd = [
        "ruff",
        "check",
        "--select",
        "F821,F401,F811,E9",
        "--config",
        'builtins = ["wmill", "context"]',
    ] + [str(f) for f in files]
    res = subprocess.run(cmd, capture_output=True, text=True)
    if res.returncode == 0:
        return []
    out = res.stdout.strip() or res.stderr.strip()
    return [out] if out else [f"ruff exited with code {res.returncode}"]


def check_python_graph(files):
    has_ruff = shutil.which("ruff") is not None
    if has_ruff:
        return ruff_check(files), "ruff"
    errors = []
    for f in files:
        errors.extend(ast_check_file(f))
    return errors, "ast"


def check_ts_graph(files, warnings):
    """Parse-check TS orchestrators with `deno fmt --check`.

    Not `deno check`: Windmill injects `windmill-client` at run time, so a type
    check from a bare checkout fails on a dependency the file is supposed to
    have. Parsing (and formatting) is the part this gate can honestly verify.
    """
    deno = shutil.which("deno")
    if not deno:
        warnings.append(
            "%d TypeScript orchestrator(s) not parse-checked (deno not installed)"
            % len(files)
        )
        return []
    errors = []
    for f in files:
        res = subprocess.run(
            [deno, "fmt", "--check", str(f)], capture_output=True, text=True
        )
        if res.returncode != 0:
            detail = (res.stderr or res.stdout).strip()
            errors.append(f"{f}: deno fmt --check failed:\n{_indent(detail)}")
    return errors


# --------------------------------------------------------------------------- #
# Flow -> script bindings
# --------------------------------------------------------------------------- #


def _inline_target(value):
    content = value.get("content")
    if not isinstance(content, str):
        return None
    if content.startswith("!inline "):
        return content[len("!inline ") :].strip()
    return None


def check_flow_bindings(flow_path):
    if yaml is None:
        return []

    errors = []
    try:
        doc = yaml.load(flow_path.read_text(encoding="utf-8"), Loader=FlowLoader)
    except Exception as e:
        return [f"{flow_path}: invalid YAML: {e}"]

    if not isinstance(doc, dict):
        return []

    modules = (doc.get("value") or {}).get("modules") or []
    flow_dir = flow_path.parent

    for idx, mod in enumerate(modules):
        mod_val = mod.get("value") or {}
        if mod_val.get("type") not in FLOW_TYPES:
            continue

        step_id = mod.get("id", idx)
        language = mod_val.get("language")
        script_name = _inline_target(mod_val)

        # 1. Policy: a step's logic lives in a Go node.
        if language in GRAPH_LANGUAGES:
            qualifies = False
            if script_name:
                candidate = flow_dir / script_name
                qualifies = candidate.is_file() and bool(
                    wac_orchestrator_reason(candidate)
                )
            if not qualifies:
                errors.append(
                    "%s (step %s): language '%s' is not allowed for a script node — "
                    "write the step in Go (docs/language-policy.md). TypeScript/Python "
                    "are only for a WAC orchestrator that dispatches to Go scripts "
                    "with taskScript()/taskFlow()."
                    % (flow_path, step_id, language)
                )
                continue
        elif language is not None and language != NODE_LANGUAGE:
            errors.append(
                "%s (step %s): unsupported script language '%s' — script nodes are Go "
                "in this repo (docs/language-policy.md)."
                % (flow_path, step_id, language)
            )
            continue

        if not script_name:
            # Inline source: nothing to cross-check, policy above already ran.
            continue

        script_path = flow_dir / script_name
        if not script_path.is_file():
            errors.append(
                f"{flow_path} (step {step_id}): inline script not found: {script_name}"
            )
            continue

        transforms = mod_val.get("input_transforms") or {}
        if not isinstance(transforms, dict):
            transforms = {}

        if script_path.suffix == ".go":
            errors.extend(
                _check_go_binding(flow_path, step_id, script_path, transforms)
            )
        elif script_path.suffix == ".py":
            errors.extend(
                _check_python_binding(flow_path, step_id, script_path, transforms)
            )
        elif script_path.suffix in (".ts", ".js", ".mjs"):
            if not wac_orchestrator_reason(script_path):
                errors.append(
                    "%s (step %s): %s has logic — script nodes must be Go "
                    "(docs/language-policy.md)." % (flow_path, step_id, script_name)
                )

    return errors


def _check_go_binding(flow_path, step_id, script_path, transforms):
    errors = []
    try:
        params, _, exists = parse_go_main(script_path.read_text(encoding="utf-8"))
    except OSError:
        return errors
    if not exists:
        return errors  # reported by check_go_script

    # Go has no default arguments: an input the flow does not pass silently
    # arrives as the zero value, which is exactly the class of bug a local gate
    # should catch.
    for req in params:
        if req not in transforms:
            errors.append(
                "%s (step %s): main() in %s takes '%s' but input_transforms does "
                "not provide it (Go has no defaults)" % (flow_path, step_id, script_path.name, req)
            )
    accepted = set(params)
    for t_arg in transforms:
        if t_arg not in accepted:
            errors.append(
                "%s (step %s): input_transforms provides '%s' but main() in %s does "
                "not accept it" % (flow_path, step_id, t_arg, script_path.name)
            )
    return errors


def _check_python_binding(flow_path, step_id, script_path, transforms):
    errors = []
    try:
        tree = ast.parse(script_path.read_text(encoding="utf-8"), filename=str(script_path))
    except Exception:
        return errors  # syntax errors are reported by the static check

    main_def = None
    for node in tree.body:
        if isinstance(node, ast.FunctionDef) and node.name == "main":
            main_def = node
            break
    if not main_def:
        return errors

    has_varkw = main_def.args.kwarg is not None
    all_pos_args = [arg.arg for arg in main_def.args.args]
    num_defaults = len(main_def.args.defaults)
    req_args = all_pos_args[:-num_defaults] if num_defaults > 0 else all_pos_args

    for req in req_args:
        if req not in transforms:
            errors.append(
                "%s (step %s): main() in %s requires argument '%s' but "
                "input_transforms does not provide it"
                % (flow_path, step_id, script_path.name, req)
            )

    if not has_varkw:
        accepted = set(all_pos_args + [arg.arg for arg in main_def.args.kwonlyargs])
        for t_arg in transforms:
            if t_arg not in accepted:
                errors.append(
                    "%s (step %s): input_transforms provides '%s' but main() in %s "
                    "does not accept it" % (flow_path, step_id, t_arg, script_path.name)
                )
    return errors


# --------------------------------------------------------------------------- #
# entry point
# --------------------------------------------------------------------------- #


def main():
    parser = argparse.ArgumentParser(
        description="Lint Windmill scripts (Go nodes, WAC-only TS/Python) and flow bindings."
    )
    parser.add_argument("paths", nargs="*", help="Optional files or directories to lint")
    args = parser.parse_args()

    ops_root = Path(os.environ.get("TURVEZ_OPS_DIR") or ROOT).resolve()

    if args.paths:
        targets = [Path(p).resolve() for p in args.paths]
        wm_dirs = []
        direct_files = []
        for t in targets:
            if t.is_file():
                direct_files.append(t)
            elif t.is_dir():
                wm_dirs.append(t)
    else:
        wm_dirs = find_windmill_dirs([ops_root / "clients", ops_root / "workflows"])
        direct_files = []
        if not wm_dirs:
            print("scripts: no windmill directories found — nothing to check")
            return 0

    if direct_files:
        # Explicit file arguments: lint exactly those (used by the tests).
        go_files = [p for p in direct_files if p.suffix == ".go"]
        graph_files = [p for p in direct_files if p.suffix in (".py", ".ts", ".js", ".mjs")]
        flow_yamls = [p for p in direct_files if p.name == "flow.yaml"]
        policy_errors = []
        for p in graph_files:
            if not wac_orchestrator_reason(p):
                policy_errors.append(
                    "%s: %s scripts must be Go (docs/language-policy.md)" % (p, p.suffix)
                )
    else:
        go_files = find_by_suffix(wm_dirs, ".go")
        flow_yamls = find_flow_yamls(wm_dirs)
        policy_errors, graph_files = check_language_policy(wm_dirs)

    if not go_files and not graph_files and not flow_yamls:
        print("scripts: no windmill scripts or flows found")
        return 0

    errors = list(policy_errors)
    warnings = []

    # 1. Go nodes
    for f in go_files:
        errs, warns = check_go_script(f)
        errors.extend(errs)
        warnings.extend(warns)

    # 2. Graph (WAC orchestrators only)
    python_files = [p for p in graph_files if p.suffix == ".py"]
    ts_files = [p for p in graph_files if p.suffix in (".ts", ".js", ".mjs")]
    tools = []
    if python_files:
        errs, tool = check_python_graph(python_files)
        errors.extend(errs)
        tools.append("python:%s" % tool)
    if ts_files:
        errors.extend(check_ts_graph(ts_files, warnings))
        tools.append("ts")

    # 3. Flow bindings
    for flow in flow_yamls:
        errors.extend(check_flow_bindings(flow))

    for warn in warnings:
        print("scripts: warn: %s" % warn, file=sys.stderr)

    if errors:
        print("scripts: check FAILED", file=sys.stderr)
        for err in errors:
            print("  %s" % err, file=sys.stderr)
        return 1

    summary = "%d go node(s), %d flow(s)" % (len(go_files), len(flow_yamls))
    if graph_files:
        summary += ", %d WAC orchestrator(s) [%s]" % (
            len(graph_files),
            ", ".join(tools) or "no static check",
        )
    print("scripts: ok (%s)" % summary)
    return 0


if __name__ == "__main__":
    sys.exit(main())
