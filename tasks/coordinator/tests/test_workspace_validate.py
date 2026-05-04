"""Pure-Python tests for workspace_validate (no real SSH).

We stub `_ssh` and `_pull_report` / `_check_remote_done` via monkeypatch
so the logic around dispatch/poll/parse is testable without a workspace.
"""

import json
from pathlib import Path
from types import SimpleNamespace

import pytest

from coordinator.db import empty_db, load_db, save_db, state_dir
from coordinator.schema import PendingValidation
from coordinator import workspace_validate as wv


def _fake_proc(returncode: int, stdout: str = "", stderr: str = ""):
    return SimpleNamespace(returncode=returncode, stdout=stdout, stderr=stderr)


# --- dispatch --- -----------------------------------------------------------

def test_workspace_name_derived_by_convention():
    # Any detector name maps to `workspace-evals-<detector>`. No hardcoded
    # detector list: new detectors get a matching workspace for free.
    assert wv.workspace_for_detector("scanmw") == "workspace-evals-scanmw"
    assert wv.workspace_for_detector("brand-new-detector") == "workspace-evals-brand-new-detector"


def test_dispatch_fails_soft_when_workspace_unreachable(tmp_path: Path, monkeypatch):
    """An unknown detector gets a derived workspace name; the convention
    workspace doesn't exist → ssh fails → dispatch returns None cleanly.

    The PendingValidation IS recorded (with status="failed") so there's
    an audit trail — persist-before-dispatch is the crash-safety fix.
    """
    state_dir(tmp_path).mkdir(parents=True)
    db = empty_db()
    monkeypatch.setattr(wv, "_ssh", lambda *a, **kw: _fake_proc(255, stderr="ssh: nonexistent host"))
    pv = wv.dispatch_validation(
        experiment_id="exp-0", candidate_id="c-0", detector="no-such-detector",
        db=db, root=tmp_path,
    )
    assert pv is None
    assert len(db.validations) == 1
    recorded = next(iter(db.validations.values()))
    assert recorded.status == "failed"
    assert recorded.completed_at is not None


def test_dispatch_happy_path(tmp_path: Path, monkeypatch):
    state_dir(tmp_path).mkdir(parents=True)
    db = empty_db()
    captured = {}

    def fake_ssh(host, command, check=False):
        captured["host"] = host
        captured["command"] = command
        return _fake_proc(0)

    monkeypatch.setattr(wv, "_ssh", fake_ssh)

    pv = wv.dispatch_validation(
        experiment_id="exp-0001", candidate_id="c-A", detector="scanmw",
        db=db, root=tmp_path,
    )
    assert pv is not None
    assert pv.workspace == "workspace-evals-scanmw"
    assert pv.detector == "scanmw"
    assert pv.experiment_id == "exp-0001"
    assert pv.remote_output_dir.endswith("exp-0001")
    assert pv.status == "pending"
    assert pv.id in db.validations
    assert "scanmw" in captured["command"]
    assert "tmux new-session" in captured["command"]


def test_dispatch_skips_if_workspace_busy(tmp_path: Path, monkeypatch):
    state_dir(tmp_path).mkdir(parents=True)
    db = empty_db()
    monkeypatch.setattr(wv, "_ssh", lambda *a, **kw: _fake_proc(0))

    wv.dispatch_validation(
        experiment_id="exp-0001", candidate_id="c-A", detector="scanmw",
        db=db, root=tmp_path,
    )
    pv2 = wv.dispatch_validation(
        experiment_id="exp-0002", candidate_id="c-B", detector="scanmw",
        db=db, root=tmp_path,
    )
    assert pv2 is None
    assert len(db.validations) == 1


def test_dispatch_ssh_failure_does_not_raise(tmp_path: Path, monkeypatch):
    state_dir(tmp_path).mkdir(parents=True)
    db = empty_db()
    monkeypatch.setattr(wv, "_ssh", lambda *a, **kw: _fake_proc(255, stderr="ssh: host unreachable"))

    pv = wv.dispatch_validation(
        experiment_id="exp-0001", candidate_id="c-A", detector="bocpd",
        db=db, root=tmp_path,
    )
    assert pv is None
    # Record persists with status=failed so a crash between ssh return
    # and driver's post-dispatch steps can't lose the audit trail.
    assert len(db.validations) == 1
    assert next(iter(db.validations.values())).status == "failed"


# --- poll --- --------------------------------------------------------------

def test_poll_pending_still_running(tmp_path: Path, monkeypatch):
    state_dir(tmp_path).mkdir(parents=True)
    db = empty_db()
    db.validations["v1"] = PendingValidation(
        id="v1", experiment_id="exp-0001", candidate_id="c",
        detector="scanmw", workspace="workspace-evals-scanmw",
        remote_output_dir="/tmp/x", dispatched_at="now", status="pending",
    )
    monkeypatch.setattr(wv, "_ssh", lambda *a, **kw: _fake_proc(1))
    transitioned = wv.poll_pending_validations(db, tmp_path)
    assert transitioned == []
    assert db.validations["v1"].status == "pending"


def test_poll_pulls_and_parses_report(tmp_path: Path, monkeypatch):
    state_dir(tmp_path).mkdir(parents=True)
    db = empty_db()
    db.validations["v1"] = PendingValidation(
        id="v1", experiment_id="exp-0001", candidate_id="c",
        detector="scanmw", workspace="workspace-evals-scanmw",
        remote_output_dir="/tmp/x", dispatched_at="now", status="pending",
    )
    monkeypatch.setattr(wv, "_check_remote_done", lambda pv: True)

    def fake_pull(pv, root):
        dest = root / "eval-results" / f"validation-{pv.id}"
        dest.mkdir(parents=True, exist_ok=True)
        (dest / "report.json").write_text(
            json.dumps(
                {
                    "component": "scanmw",
                    "recommendation": "keep",
                    "summary": {"delta_max": 0.07},
                }
            )
        )
        return dest

    monkeypatch.setattr(wv, "_pull_report", fake_pull)

    transitioned = wv.poll_pending_validations(db, tmp_path)
    assert transitioned == ["v1"]
    v = db.validations["v1"]
    assert v.status == "done"
    assert v.recommendation == "keep"
    assert v.delta_max == pytest.approx(0.07)
    assert v.local_path is not None
    assert v.completed_at is not None


def test_poll_handles_pull_failure(tmp_path: Path, monkeypatch):
    state_dir(tmp_path).mkdir(parents=True)
    db = empty_db()
    db.validations["v1"] = PendingValidation(
        id="v1", experiment_id="exp-0001", candidate_id="c",
        detector="scanmw", workspace="workspace-evals-scanmw",
        remote_output_dir="/tmp/x", dispatched_at="now", status="pending",
    )
    monkeypatch.setattr(wv, "_check_remote_done", lambda pv: True)
    monkeypatch.setattr(wv, "_pull_report", lambda pv, root: None)

    transitioned = wv.poll_pending_validations(db, tmp_path)
    assert transitioned == []
    assert db.validations["v1"].status == "pending"


def test_validations_roundtrip_through_yaml(tmp_path: Path):
    db = empty_db()
    db.validations["v-abc"] = PendingValidation(
        id="v-abc",
        experiment_id="exp-0001",
        candidate_id="A-tighten-scan-gate",
        detector="scanmw",
        workspace="workspace-evals-scanmw",
        remote_output_dir="/tmp/observer-component-eval/exp-0001",
        dispatched_at="2026-04-21T10:00:00",
        status="done",
        completed_at="2026-04-21T13:00:00",
        local_path="eval-results/validation-v-abc",
        recommendation="keep",
        delta_max=0.085,
    )
    save_db(db, tmp_path)
    reloaded = load_db(tmp_path)
    v = reloaded.validations["v-abc"]
    assert v.recommendation == "keep"
    assert v.delta_max == pytest.approx(0.085)
    assert v.workspace == "workspace-evals-scanmw"
