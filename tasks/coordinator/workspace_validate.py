"""Post-ship workspace validation: fire-and-forget async `q.eval-component`.

After a candidate ships, dispatch an eval-component run on the matching
workspace (one dedicated workspace per detector, per user convention:
`evals-bocpd`, `evals-scanmw`, `evals-scanwelch`). The coordinator does
NOT block: the run happens asynchronously in a tmux session on the
workspace. At iteration start we poll; when `report.json` lands on the
remote, we scp it back and record the findings on the experiment.

This is a lagging data point, not a gate. Downstream coordinator
decisions never depend on validation results. Useful for human audit
after the fact.
"""

from __future__ import annotations

import datetime as _dt
import json
import shlex
import subprocess
import uuid
from pathlib import Path

from . import coord_out, journal
from .config import CONFIG
from .schema import Db, PendingValidation


# Convention: one dedicated validation workspace per detector, named
# `workspace-evals-<detector>`. Any new detector gets a matching
# auto-derived workspace name — no code change needed here. If the
# derived workspace doesn't exist on ssh, dispatch fails soft
# (journalled, returns None) and the coordinator moves on.
WORKSPACE_PREFIX = "workspace-evals-"


def workspace_for_detector(detector: str) -> str:
    """Map a detector name to its validation workspace by convention.

    Override this function (or monkeypatch it in tests) if a detector
    ever needs a non-conventional workspace.
    """
    return f"{WORKSPACE_PREFIX}{detector}"


def _now() -> str:
    return _dt.datetime.now().isoformat(timespec="seconds")


# Timeout on every ssh invocation so a half-open TCP connection to a
# paused/reaped workspace cannot wedge the main loop indefinitely. The
# -o flags bound connect + liveness so a silent peer is detected in
# ~45s rather than TCP's default 2hr.
_SSH_OPTS = [
    "-o", "ConnectTimeout=10",
    "-o", "ServerAliveInterval=15",
    "-o", "ServerAliveCountMax=3",
    "-o", "BatchMode=yes",
]
_SSH_TIMEOUT_SECONDS = 60


def _ssh(host: str, command: str, check: bool = False) -> subprocess.CompletedProcess:
    try:
        return subprocess.run(
            ["ssh", *_SSH_OPTS, host, command],
            capture_output=True,
            text=True,
            check=check,
            timeout=_SSH_TIMEOUT_SECONDS,
        )
    except subprocess.TimeoutExpired as e:
        # Synthesize a non-zero-returncode CompletedProcess so callers keep
        # their (rc, stderr) contract. Raising would escape fail-soft paths.
        return subprocess.CompletedProcess(
            args=e.cmd, returncode=124,
            stdout="", stderr=f"ssh timeout after {_SSH_TIMEOUT_SECONDS}s",
        )


def workspace_busy(db: Db, workspace: str) -> bool:
    """Already running a validation on this workspace?"""
    return any(
        v.workspace == workspace and v.status in {"pending", "dispatching"}
        for v in db.validations.values()
    )


def dispatch_validation(
    experiment_id: str,
    candidate_id: str,
    detector: str,
    db: Db,
    root: Path,
) -> PendingValidation | None:
    """Start an eval-component run on the detector's dedicated workspace.

    Returns the PendingValidation record (also inserted into db.validations),
    or None if no matching workspace or workspace is busy. Never raises on
    network/ssh error — a dispatch failure is recorded in the journal and
    the coordinator continues.
    """
    workspace = workspace_for_detector(detector)
    if workspace_busy(db, workspace):
        journal.append(
            "validation_skipped_busy",
            {"experiment_id": experiment_id, "workspace": workspace},
            root,
        )
        return None

    vid = f"val-{uuid.uuid4().hex[:8]}"
    remote_dir = f"/tmp/observer-component-eval/{experiment_id}"

    # Persist the record BEFORE ssh so a crash between ssh return and
    # db update can't lose track of the in-flight remote tmux session.
    # Starts as `dispatching`; flips to `pending` on ssh success, `failed`
    # on ssh failure. `workspace_busy` treats `dispatching` same as pending.
    pv = PendingValidation(
        id=vid,
        experiment_id=experiment_id,
        candidate_id=candidate_id,
        detector=detector,
        workspace=workspace,
        remote_output_dir=remote_dir,
        dispatched_at=_now(),
        status="dispatching",
    )
    db.validations[vid] = pv
    # Lazy import to avoid db → workspace_validate cycle.
    from .db import save_db

    save_db(db, root)
    journal.append(
        "validation_dispatching",
        {"validation_id": vid, "experiment_id": experiment_id, "workspace": workspace, "detector": detector},
        root,
    )

    # Quoting discipline: build the payload as a plain string then
    # shlex.quote the entire thing as one tmux arg (avoids the nested-
    # single-quote trap if a future input ever contains a literal quote).
    inner = (
        f"cd ~/datadog-agent && "
        f"dda inv --dep=optuna q.eval-component "
        f"--component {detector} "
        f"--output-dir {remote_dir}"
    )
    cmd = f"tmux new-session -d -s {shlex.quote(vid)} {shlex.quote(inner)}"
    res = _ssh(workspace, cmd)
    if res.returncode != 0:
        pv.status = "failed"
        pv.completed_at = _now()
        save_db(db, root)
        journal.append(
            "validation_dispatch_failed",
            {"validation_id": vid, "workspace": workspace, "stderr": res.stderr[-500:]},
            root,
        )
        return None

    pv.status = "pending"
    save_db(db, root)
    journal.append(
        "validation_dispatched",
        {"validation_id": vid, "experiment_id": experiment_id, "workspace": workspace, "detector": detector},
        root,
    )
    return pv


def _check_remote_done(pv: PendingValidation) -> bool:
    """Is `report.json` present on the remote workspace yet?"""
    res = _ssh(pv.workspace, f"test -f {shlex.quote(pv.remote_output_dir)}/report.json")
    return res.returncode == 0


def _pull_report(pv: PendingValidation, root: Path) -> Path | None:
    """scp the remote output dir to local eval-results/. Returns local dir."""
    local_dir = root / "eval-results" / f"validation-{pv.id}"
    local_dir.mkdir(parents=True, exist_ok=True)
    try:
        res = subprocess.run(
            [
                "scp",
                *_SSH_OPTS,
                "-r",
                f"{pv.workspace}:{pv.remote_output_dir}/.",
                str(local_dir) + "/",
            ],
            capture_output=True,
            text=True,
            timeout=300,  # report dirs are small; 5min is generous.
        )
    except subprocess.TimeoutExpired:
        return None
    if res.returncode != 0:
        return None
    return local_dir


def _parse_report(report_path: Path) -> tuple[str | None, float | None]:
    try:
        with report_path.open() as f:
            r = json.load(f)
    except (OSError, json.JSONDecodeError):
        return None, None
    reco = r.get("recommendation")
    summary = r.get("summary") or {}
    delta_max = summary.get("delta_max")
    return reco, delta_max


def _age_hours(dispatched_at: str) -> float | None:
    try:
        ts = _dt.datetime.fromisoformat(dispatched_at)
    except ValueError:
        return None
    return (_dt.datetime.now() - ts).total_seconds() / 3600.0


def poll_pending_validations(db: Db, root: Path) -> list[str]:
    """Check each pending validation; pull + record any that have landed.

    Returns the list of validation ids that transitioned to done this call.
    Intended to be called at iteration start; short-circuits on ssh errors.

    Abandons (marks status=abandoned) validations older than
    CONFIG.validation_max_age_hours without a landed report.
    """
    transitioned: list[str] = []
    for pv in list(db.validations.values()):
        if pv.status != "pending":
            continue
        # Abandon stale validations; workspace may have been killed / reimaged.
        age = _age_hours(pv.dispatched_at)
        if age is not None and age > CONFIG.validation_max_age_hours:
            pv.status = "abandoned"
            pv.completed_at = _now()
            journal.append(
                "validation_abandoned",
                {"validation_id": pv.id, "age_hours": age},
                root,
            )
            coord_out.emit(
                "validation_abandoned",
                f"Validation `{pv.id}` ({pv.detector}) was pending for "
                f"{age:.1f}h without landing; marked abandoned.",
                requires_ack=False,
                root=root,
            )
            continue
        try:
            done = _check_remote_done(pv)
        except Exception as e:
            journal.append(
                "validation_poll_error",
                {"validation_id": pv.id, "error": str(e)},
                root,
            )
            continue
        if not done:
            continue
        local = _pull_report(pv, root)
        if local is None:
            journal.append(
                "validation_pull_failed",
                {"validation_id": pv.id, "workspace": pv.workspace},
                root,
            )
            continue
        reco, delta_max = _parse_report(local / "report.json")
        pv.status = "done"
        pv.completed_at = _now()
        pv.local_path = str(local)
        pv.recommendation = reco
        pv.delta_max = delta_max
        transitioned.append(pv.id)
        journal.append(
            "validation_completed",
            {
                "validation_id": pv.id,
                "experiment_id": pv.experiment_id,
                "recommendation": reco,
                "delta_max": delta_max,
            },
            root,
        )
        delta_str = f"{delta_max:.4f}" if delta_max is not None else "—"
        coord_out.emit(
            "validation_completed",
            f"Validation `{pv.id}` for experiment `{pv.experiment_id}` "
            f"(candidate `{pv.candidate_id}`, detector `{pv.detector}`) finished. "
            f"Recommendation: **{reco or '—'}**, Δmax={delta_str}. "
            f"Local: {local}/",
            requires_ack=False,
            root=root,
        )
    return transitioned
