# ABOUTME: Unit tests for muttest_render — gremlins-JSON → markdown rendering.
from __future__ import annotations

import importlib.util
import json
from pathlib import Path
import sys

HERE = Path(__file__).resolve().parent
RENDERER = HERE.parent / "muttest_render.py"

spec = importlib.util.spec_from_file_location("muttest_render", RENDERER)
assert spec is not None and spec.loader is not None, f"Could not load module from {RENDERER}"
muttest_render = importlib.util.module_from_spec(spec)
sys.modules["muttest_render"] = muttest_render
spec.loader.exec_module(muttest_render)

MAX_COMMENT_BYTES = muttest_render.MAX_COMMENT_BYTES
_load_mutants = muttest_render._load_mutants
_render_report = muttest_render._render_report


def _write_gremlins_json(tmp_path: Path, payload: dict, name: str = "pkg.json") -> Path:
    f = tmp_path / name
    f.write_text(json.dumps(payload))
    return f


def test_empty_results_dir_returns_empty_list(tmp_path):
    assert _load_mutants(tmp_path) == []


def test_load_mutants_parses_gremlins_shape(tmp_path):
    _write_gremlins_json(
        tmp_path,
        {
            "files": [
                {
                    "file_name": "foo.go",
                    "mutations": [
                        {
                            "line": 10,
                            "column": 3,
                            "status": "KILLED",
                            "type": "CONDITIONALS_BOUNDARY",
                            "diff": "-a\n+b",
                        },
                        {"line": 12, "column": 1, "status": "LIVED", "type": "INCREMENT_DECREMENT", "diff": "-c\n+d"},
                    ],
                }
            ],
        },
    )
    mutants = _load_mutants(tmp_path)
    assert len(mutants) == 2
    assert mutants[0]["file"] == "foo.go"
    assert mutants[0]["status"] == "KILLED"
    assert mutants[1]["status"] == "LIVED"


def test_render_reports_correct_score(tmp_path):
    mutants = [
        {"file": "f.go", "line": 1, "column": 1, "status": "KILLED", "type": "X", "diff": ""},
        {"file": "f.go", "line": 2, "column": 1, "status": "LIVED", "type": "X", "diff": ""},
        {"file": "f.go", "line": 3, "column": 1, "status": "NOT_COVERED", "type": "X", "diff": ""},
        {"file": "f.go", "line": 4, "column": 1, "status": "NOT_VIABLE", "type": "X", "diff": ""},
    ]
    out = _render_report(mutants, local_cmd="run-it")
    # 1 killed, 2 survived (LIVED + NOT_COVERED), 1 other → 25% score
    assert "Score: 25.0%" in out
    assert "2 surviving mutant" in out
    assert "run-it" in out


def test_load_mutants_ignores_non_json_files(tmp_path):
    (tmp_path / "results.txt").write_text('{"files": []}')
    (tmp_path / "notes.md").write_text("not json")
    assert _load_mutants(tmp_path) == []


def test_load_mutants_skips_malformed_json(tmp_path):
    (tmp_path / "bad.json").write_text("not valid json {{{")
    _write_gremlins_json(
        tmp_path,
        {
            "files": [
                {
                    "file_name": "f.go",
                    "mutations": [{"line": 1, "column": 1, "status": "KILLED", "type": "X", "diff": ""}],
                }
            ]
        },
        name="good.json",
    )
    mutants = _load_mutants(tmp_path)
    assert len(mutants) == 1


def test_render_truncation_fits_under_cap():
    # Enough mutants to blow past the size cap; the ladder must find a level that fits.
    mutants = [
        {
            "file": f"file_{i // 5}.go",
            "line": i,
            "column": 1,
            "status": "LIVED",
            "type": "CONDITIONALS_BOUNDARY",
            "diff": "x" * 200,
        }
        for i in range(5000)
    ]
    out = _render_report(mutants, local_cmd="run-it")
    assert "per-mutant details omitted" in out
    assert len(out.encode("utf-8")) <= MAX_COMMENT_BYTES
