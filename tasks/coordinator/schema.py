from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Any


class Phase(str, Enum):
    ZERO = "0"
    ONE = "1"
    TWO = "2"
    THREE = "3"
    FOUR = "4"


class Tier(str, Enum):
    T0 = "t0"
    T1 = "t1"
    T2 = "t2"
    T3 = "t3"
    T4 = "t4"


class ExperimentStatus(str, Enum):
    PENDING = "pending"
    DISPATCHED = "dispatched"
    RUNNING = "running"
    DONE = "done"
    FAILED = "failed"
    TIMED_OUT = "timed_out"


class CandidateStatus(str, Enum):
    """Candidate lifecycle.

    Simplified per allium rev-8: gates (strict-regression, review) run
    deterministically within one iteration, so intermediate states like
    t0_passed/escalated are collapsed — a candidate is either active
    (PROPOSED) or terminal (SHIPPED/REJECTED).
    """

    PROPOSED = "proposed"
    SHIPPED = "shipped"
    REJECTED = "rejected"


@dataclass
class ScenarioResult:
    f1: float
    precision: float
    recall: float
    num_baseline_fps: int
    # Per-scenario F1 σ measured across replicate runs. 0.0 = unknown
    # (single-seed baseline); callers fall back to CONFIG.tau_default.
    # Populated by `measure_sigma.py` after a multi-seed rebench.
    f1_sigma: float = 0.0


# Renamed-by-purpose: BaselineMetrics describes the system as a whole now,
# not one detector. Kept the old `BaselineDetector` symbol as an alias so
# existing imports / tests don't break. The internal shape is unchanged
# (mean_f1, total_fps, per-scenario dict).
@dataclass
class BaselineMetrics:
    mean_f1: float
    total_fps: int
    scenarios: dict[str, ScenarioResult]


# Legacy alias.
BaselineDetector = BaselineMetrics


@dataclass
class Baseline:
    """System-level baseline. One pipeline, one eval, one record.

    Previously this was a per-detector dict — scoring compared each
    candidate against `baseline.detectors[<detector_name>]`. That model
    doesn't generalize to correlators/filters (no standalone baseline)
    and quietly mismeasured detector candidates too (per-detector F1
    differs from system F1 once detectors interact). The system-level
    pipeline (catalog defaults, run via `q.eval-scenarios` with no
    --only) is what production actually runs and what should be compared.
    """

    sha: str
    generated_at: str
    system: BaselineMetrics


@dataclass
class Candidate:
    id: str
    description: str
    source: str  # "seed" | "coordinator-proposed"
    target_components: list[str]
    phase: Phase
    status: CandidateStatus = CandidateStatus.PROPOSED
    proposed_at: str = ""
    # Diversity policy: coordinator uses approach_family to detect when it's
    # stuck on one approach. Free-form short string, e.g. "threshold-tune",
    # "anomaly-rank-filter", "detector-swap", "correlator-new".
    approach_family: str = "unspecified"
    # Which prior candidate IDs informed this one (proposer fills this in).
    parent_candidates: list[str] = field(default_factory=list)
    # Detailed implementation plan authored by the proposer (Opus). The
    # implementer (Sonnet) follows it mechanically rather than redesigning
    # in-flight — that's the split that keeps Task-tool amplification and
    # context exhaustion out of the implementation call. Reviewer compares
    # the actual diff against this plan for "plan_fidelity": deviations
    # are flagged but a net-positive deviation is approvable.
    implementation_plan: str = ""


class RejectionStage(str, Enum):
    """Where in the iteration pipeline a candidate was rejected.

    Single enumeration of every "this candidate dies here" point so
    "why was this rejected?" is a single-query answer instead of a
    grep across driver.py.
    """

    REGISTRATION = "registration"          # detector not in component_catalog.go
    PRE_EVAL_GATE = "pre_eval_gate"        # independent reviewer predicts eval is wasteful
    EVAL_FAILED = "eval_failed"            # evaluator returned non-ok
    EVAL_SILENT_FAILURE = "eval_silent_failure"  # all-zero metrics
    TIER1_GATE = "tier1_gate"              # egregious gate (FPs > 3× + abs floor, first-ship)
    REVIEW = "review"                      # 3-persona panel said no
    REVIEW_CRASHED = "review_crashed"      # review SDK call crashed


@dataclass
class RejectionDecision:
    """Structured record of why a candidate was rejected.

    One per rejection. Emitted by `driver._reject_candidate()` (the
    single rejection sink) so every decision has the same shape:
    stage + reason + evidence dict + any tier-2 advisory signals
    that influenced the call (or were noted but didn't fire).

    `evidence` is a free-form dict for stage-specific data
    (e.g. {"fp_observed": 87, "fp_ceiling": 60, "baseline_fps": 20}
    for tier1 FP-egregious, or {"unregistered": [...]} for
    registration). Persisted on the experiment via
    `auto_reject_reason` (compact string) plus this dict via the
    journal.
    """

    stage: RejectionStage
    reason: str
    evidence: dict[str, Any] = field(default_factory=dict)
    advisory_signals: list[str] = field(default_factory=list)


@dataclass
class ReviewDecision:
    persona: str
    approve: bool
    rationale: str


@dataclass
class ReviewVerdict:
    unanimous_approve: bool
    decisions: list[ReviewDecision] = field(default_factory=list)


@dataclass
class Experiment:
    id: str
    candidate_id: str
    phase: Phase
    tier: Tier
    commit_sha: str
    config_path: str
    scenario_set: list[str]
    status: ExperimentStatus = ExperimentStatus.PENDING
    workspace: str | None = None
    started_at: str | None = None
    completed_at: str | None = None
    report_path: str | None = None
    score: float | None = None
    num_baseline_fps_sum: int | None = None
    per_scenario: dict[str, ScenarioResult] = field(default_factory=dict)
    review: ReviewVerdict | None = None
    # Signal for the proposer's "research memory" on future iterations —
    # without these, iter N+1's proposer only sees aggregate score_delta
    # and redacted rationales, losing the per-scenario failure pattern.
    impl_summary: str = ""           # the DONE: line from the impl agent
    auto_reject_reason: str = ""     # populated when gate rejected pre-review


@dataclass
class PhaseState:
    current_phase: Phase
    best_score: float
    plateau_counter: int
    phase_start_iter: int


@dataclass
class BudgetState:
    wall_hours_used: float
    wall_hours_ceiling: float | None   # set after M0.2
    api_tokens_used: int                # cached total; authoritative source
                                        # is .coordinator/tokens.jsonl
    api_token_ceiling: int | None
    milestones_notified: list[float] = field(default_factory=list)
    # Streak of consecutive iterations whose cost was anomalous (per
    # CONFIG.cost_anomaly_*). At cost_anomaly_pause_streak, driver
    # touches .coordinator/pause to halt cooperatively at iter boundary.
    consecutive_cost_anomalies: int = 0
    # Streak of consecutive iterations whose eval reported all-zero metrics.
    # Single silent_failure now rejects-and-continues (the candidate likely
    # had a structural issue — e.g. correlator run standalone — not the
    # eval pipeline). Only at silent_failure_pause_streak does the driver
    # auto-pause, on the assumption the eval environment is genuinely sick.
    # Resets to 0 on the first iter that produces non-zero metrics.
    consecutive_silent_failures: int = 0
    # Streak of consecutive iterations where the pre-eval sanity sentinel
    # failed. Single failure skips the iter and continues (the candidate
    # likely modified the sentinel detector). Only at streak-3 does the
    # driver auto-pause on the assumption the env is genuinely broken.
    consecutive_sentinel_failures: int = 0


@dataclass
class PendingValidation:
    """A post-ship workspace validation (e.g. q.eval-component).

    Dispatched fire-and-forget on a dedicated workspace after a candidate
    ships. Polled at iteration start; when the remote `report.json` lands,
    its contents are scp'd back and summarized onto the experiment record.
    Never gates or influences downstream coordinator decisions — it is a
    lagging confirmation artefact.
    """

    id: str
    experiment_id: str
    candidate_id: str
    detector: str
    workspace: str  # SSH alias, e.g. "workspace-evals-scanmw"
    remote_output_dir: str
    dispatched_at: str
    status: str = "pending"  # pending | done | failed
    completed_at: str | None = None
    local_path: str | None = None
    recommendation: str | None = None  # "keep" / "discard" once parsed
    delta_max: float | None = None


@dataclass
class Iteration:
    number: int
    started_at: str
    ended_at: str | None = None
    candidate_id: str | None = None
    experiment_ids: list[str] = field(default_factory=list)
    inbox_acks: list[str] = field(default_factory=list)  # ack log ids


@dataclass
class DataSplit:
    """Train/lockbox partition of the scenario corpus.

    `sealed_hash` is a deterministic hash of the lockbox membership.
    Changing lockbox membership invalidates the hash and requires an
    explicit user ack (via inbox) before proceeding.
    """

    train: list[str]
    lockbox: list[str]
    sealed_hash: str

    def as_train_set(self) -> set[str]:
        return set(self.train)

    def as_lockbox_set(self) -> set[str]:
        return set(self.lockbox)


@dataclass
class Db:
    schema_version: int
    baseline: Baseline | None
    experiments: dict[str, Experiment]
    candidates: dict[str, Candidate]
    phase_state: PhaseState
    budget: BudgetState
    iterations: list[Iteration]
    split: DataSplit | None = None
    validations: dict[str, PendingValidation] = field(default_factory=dict)
    # Components for which eval-component has been dispatched at least once.
    # Prevents re-running on every ship; eval-component is a "certify this
    # new component" step, not a per-config check.
    components_eval_dispatched: list[str] = field(default_factory=list)

    # Approach_families permanently banned after a phase-plateau pivot
    # cleared them out. Persistent across restarts. Proposer reads this
    # and treats these as "don't revisit" for the rest of the run.
    # Fully autonomous self-redirection: no user input required.
    pivot_banned_families: list[str] = field(default_factory=list)
    # Number of times the coordinator has auto-pivoted (for bookkeeping
    # + the hard-runaway exit check at `max_pivots_before_halt`).
    pivot_count: int = 0

    # Operator steering directives, accumulated from inbox_ack interpretations.
    # The proposer reads these on every invocation and treats them as
    # NON-NEGOTIABLE constraints, not advisory text. Earlier the inbox loop
    # interpreted user comments and journaled them but never fed them back
    # into the proposer prompt — user steering disappeared into the
    # journal. Adding this field closes the loop.
    user_steering_active: list[str] = field(default_factory=list)

    # DEPRECATED — rolling-reference mechanism dropped (introduced a
    # noise-driven ratchet that let candidates strictly worse than baseline
    # ship). Field kept so old db.yaml files still load; never written.
    # Remove after all active runs migrate.
    last_shipped_per_scenario: dict[str, dict[str, ScenarioResult]] = field(default_factory=dict)

    # Best-historical effective baseline. Populated post-ship by element-
    # wise max(f1, precision, recall) and min(num_baseline_fps) across the
    # original baseline + every prior ship. None → use db.baseline directly.
    #
    # Solves the noise-ratchet that doomed `last_shipped_per_scenario`:
    # the previous design used last-shipped AS the reference, which meant
    # a ship with F1=0.49 (vs baseline 0.50) could drift through subsequent
    # iterations until cumulative drops put cleanly below baseline. Best-
    # historical can only ratchet UP (or sideways), never down — gates
    # always compare against the highest score we've ever achieved.
    #
    # On blank-mode runs where original baseline is all zeros, this gives
    # subsequent iters a real floor as soon as the first ship lands.
    effective_baseline: Baseline | None = None

    # Content-hash snapshot of PROTECTED_PATHS (scoring labels). Verified at
    # every iteration start; halt on mismatch (agent or human tampered with
    # ground truth). Empty dict on first run = bootstrap, take snapshot.
    protected_path_hashes: dict[str, str] = field(default_factory=dict)


def dict_to_db(d: dict[str, Any]) -> Db:
    """Reconstruct Db from a dict (loaded from YAML)."""

    def _scenario(x: dict) -> ScenarioResult:
        return ScenarioResult(
            f1=x["f1"],
            precision=x["precision"],
            recall=x["recall"],
            num_baseline_fps=x["num_baseline_fps"],
            f1_sigma=float(x.get("f1_sigma", 0.0) or 0.0),
        )

    def _baseline_from_dict(b: dict | None) -> Baseline | None:
        if not b:
            return None
        # New format: {"system": {mean_f1, total_fps, scenarios}}
        # Old format: {"detectors": {<name>: {...}, ...}} — operator must
        # re-run import_baseline after upgrade. We pick first detector as a
        # stand-in (warning printed) so the driver doesn't crash.
        if "system" in b:
            sys = b["system"]
            metrics = BaselineMetrics(
                mean_f1=sys["mean_f1"],
                total_fps=sys["total_fps"],
                scenarios={s: _scenario(sr) for s, sr in sys["scenarios"].items()},
            )
        elif "detectors" in b:
            import sys as _sys
            print(
                "WARNING: legacy per-detector baseline. Re-run "
                "`coordinator.import_baseline --report <system_report.json>`. "
                "Using first detector's record as a stand-in.",
                file=_sys.stderr,
            )
            first_det = next(iter(b["detectors"].values()), None)
            if first_det is None:
                return None
            metrics = BaselineMetrics(
                mean_f1=first_det["mean_f1"],
                total_fps=first_det["total_fps"],
                scenarios={s: _scenario(sr) for s, sr in first_det["scenarios"].items()},
            )
        else:
            return None
        return Baseline(
            sha=b["sha"],
            generated_at=b["generated_at"],
            system=metrics,
        )

    baseline = _baseline_from_dict(d.get("baseline"))
    effective_baseline = _baseline_from_dict(d.get("effective_baseline"))

    experiments = {}
    for eid, e in d.get("experiments", {}).items():
        per_scen = {s: _scenario(sr) for s, sr in (e.get("per_scenario") or {}).items()}
        rev = None
        if e.get("review"):
            rev = ReviewVerdict(
                unanimous_approve=e["review"]["unanimous_approve"],
                decisions=[
                    ReviewDecision(**dd) for dd in e["review"].get("decisions", [])
                ],
            )
        experiments[eid] = Experiment(
            id=e["id"],
            candidate_id=e["candidate_id"],
            phase=Phase(e["phase"]),
            tier=Tier(e["tier"]),
            commit_sha=e["commit_sha"],
            config_path=e["config_path"],
            scenario_set=e["scenario_set"],
            status=ExperimentStatus(e["status"]),
            workspace=e.get("workspace"),
            started_at=e.get("started_at"),
            completed_at=e.get("completed_at"),
            report_path=e.get("report_path"),
            score=e.get("score"),
            num_baseline_fps_sum=e.get("num_baseline_fps_sum"),
            per_scenario=per_scen,
            review=rev,
            impl_summary=e.get("impl_summary", "") or "",
            auto_reject_reason=e.get("auto_reject_reason", "") or "",
        )

    candidates = {
        cid: Candidate(
            id=c["id"],
            description=c["description"],
            source=c["source"],
            target_components=c.get("target_components", []),
            phase=Phase(c["phase"]),
            status=CandidateStatus(c.get("status", "proposed")),
            proposed_at=c.get("proposed_at", ""),
            approach_family=c.get("approach_family", "unspecified"),
            parent_candidates=list(c.get("parent_candidates", [])),
            implementation_plan=c.get("implementation_plan", "") or "",
        )
        for cid, c in d.get("candidates", {}).items()
    }

    ps = d["phase_state"]
    bs = d["budget"]
    split = None
    if d.get("split"):
        s = d["split"]
        split = DataSplit(
            train=list(s["train"]),
            lockbox=list(s["lockbox"]),
            sealed_hash=s["sealed_hash"],
        )

    validations = {}
    for vid, v in (d.get("validations") or {}).items():
        validations[vid] = PendingValidation(
            id=v["id"],
            experiment_id=v["experiment_id"],
            candidate_id=v["candidate_id"],
            detector=v["detector"],
            workspace=v["workspace"],
            remote_output_dir=v["remote_output_dir"],
            dispatched_at=v["dispatched_at"],
            status=v.get("status", "pending"),
            completed_at=v.get("completed_at"),
            local_path=v.get("local_path"),
            recommendation=v.get("recommendation"),
            delta_max=v.get("delta_max"),
        )

    components_eval_dispatched = list(d.get("components_eval_dispatched") or [])

    last_shipped: dict[str, dict[str, ScenarioResult]] = {}
    for det_name, scens in (d.get("last_shipped_per_scenario") or {}).items():
        last_shipped[det_name] = {s: _scenario(sr) for s, sr in scens.items()}

    return Db(
        schema_version=d["schema_version"],
        baseline=baseline,
        experiments=experiments,
        candidates=candidates,
        phase_state=PhaseState(
            current_phase=Phase(ps["current_phase"]),
            best_score=ps["best_score"],
            plateau_counter=ps["plateau_counter"],
            phase_start_iter=ps["phase_start_iter"],
        ),
        budget=BudgetState(
            wall_hours_used=bs["wall_hours_used"],
            wall_hours_ceiling=bs.get("wall_hours_ceiling"),
            api_tokens_used=bs["api_tokens_used"],
            api_token_ceiling=bs.get("api_token_ceiling"),
            milestones_notified=bs.get("milestones_notified", []),
            consecutive_cost_anomalies=int(bs.get("consecutive_cost_anomalies", 0) or 0),
            consecutive_silent_failures=int(bs.get("consecutive_silent_failures", 0) or 0),
            consecutive_sentinel_failures=int(bs.get("consecutive_sentinel_failures", 0) or 0),
        ),
        iterations=[Iteration(**it) for it in d.get("iterations", [])],
        split=split,
        validations=validations,
        components_eval_dispatched=components_eval_dispatched,
        last_shipped_per_scenario=last_shipped,
        effective_baseline=effective_baseline,
        protected_path_hashes=dict(d.get("protected_path_hashes") or {}),
        pivot_banned_families=list(d.get("pivot_banned_families") or []),
        pivot_count=int(d.get("pivot_count", 0) or 0),
        user_steering_active=list(d.get("user_steering_active") or []),
    )


def db_to_dict(db: Db) -> dict[str, Any]:
    def _s(s: ScenarioResult) -> dict:
        return {
            "f1": s.f1,
            "precision": s.precision,
            "recall": s.recall,
            "num_baseline_fps": s.num_baseline_fps,
            "f1_sigma": s.f1_sigma,
        }

    def _baseline_to_dict(b: Baseline | None) -> dict | None:
        if not b:
            return None
        return {
            "sha": b.sha,
            "generated_at": b.generated_at,
            "system": {
                "mean_f1": b.system.mean_f1,
                "total_fps": b.system.total_fps,
                "scenarios": {s: _s(sr) for s, sr in b.system.scenarios.items()},
            },
        }

    baseline_d = _baseline_to_dict(db.baseline)
    effective_baseline_d = _baseline_to_dict(db.effective_baseline)

    return {
        "schema_version": db.schema_version,
        "baseline": baseline_d,
        "effective_baseline": effective_baseline_d,
        "experiments": {
            eid: {
                "id": e.id,
                "candidate_id": e.candidate_id,
                "phase": e.phase.value,
                "tier": e.tier.value,
                "commit_sha": e.commit_sha,
                "config_path": e.config_path,
                "scenario_set": e.scenario_set,
                "status": e.status.value,
                "workspace": e.workspace,
                "started_at": e.started_at,
                "completed_at": e.completed_at,
                "report_path": e.report_path,
                "score": e.score,
                "num_baseline_fps_sum": e.num_baseline_fps_sum,
                "per_scenario": {s: _s(sr) for s, sr in e.per_scenario.items()},
                "review": (
                    {
                        "unanimous_approve": e.review.unanimous_approve,
                        "decisions": [
                            {"persona": d.persona, "approve": d.approve, "rationale": d.rationale}
                            for d in e.review.decisions
                        ],
                    }
                    if e.review
                    else None
                ),
                "impl_summary": e.impl_summary,
                "auto_reject_reason": e.auto_reject_reason,
            }
            for eid, e in db.experiments.items()
        },
        "candidates": {
            cid: {
                "id": c.id,
                "description": c.description,
                "source": c.source,
                "target_components": c.target_components,
                "phase": c.phase.value,
                "status": c.status.value,
                "proposed_at": c.proposed_at,
                "approach_family": c.approach_family,
                "parent_candidates": c.parent_candidates,
                "implementation_plan": c.implementation_plan,
            }
            for cid, c in db.candidates.items()
        },
        "phase_state": {
            "current_phase": db.phase_state.current_phase.value,
            "best_score": db.phase_state.best_score,
            "plateau_counter": db.phase_state.plateau_counter,
            "phase_start_iter": db.phase_state.phase_start_iter,
        },
        "budget": {
            "wall_hours_used": db.budget.wall_hours_used,
            "wall_hours_ceiling": db.budget.wall_hours_ceiling,
            "api_tokens_used": db.budget.api_tokens_used,
            "api_token_ceiling": db.budget.api_token_ceiling,
            "milestones_notified": db.budget.milestones_notified,
            "consecutive_cost_anomalies": db.budget.consecutive_cost_anomalies,
            "consecutive_silent_failures": db.budget.consecutive_silent_failures,
            "consecutive_sentinel_failures": db.budget.consecutive_sentinel_failures,
        },
        "iterations": [
            {
                "number": it.number,
                "started_at": it.started_at,
                "ended_at": it.ended_at,
                "candidate_id": it.candidate_id,
                "experiment_ids": it.experiment_ids,
                "inbox_acks": it.inbox_acks,
            }
            for it in db.iterations
        ],
        "split": (
            {
                "train": db.split.train,
                "lockbox": db.split.lockbox,
                "sealed_hash": db.split.sealed_hash,
            }
            if db.split
            else None
        ),
        "components_eval_dispatched": db.components_eval_dispatched,
        "pivot_banned_families": list(db.pivot_banned_families),
        "pivot_count": db.pivot_count,
        "user_steering_active": list(db.user_steering_active),
        "last_shipped_per_scenario": {
            det: {s: _s(sr) for s, sr in scens.items()}
            for det, scens in db.last_shipped_per_scenario.items()
        },
        "protected_path_hashes": dict(db.protected_path_hashes),
        "validations": {
            vid: {
                "id": v.id,
                "experiment_id": v.experiment_id,
                "candidate_id": v.candidate_id,
                "detector": v.detector,
                "workspace": v.workspace,
                "remote_output_dir": v.remote_output_dir,
                "dispatched_at": v.dispatched_at,
                "status": v.status,
                "completed_at": v.completed_at,
                "local_path": v.local_path,
                "recommendation": v.recommendation,
                "delta_max": v.delta_max,
            }
            for vid, v in db.validations.items()
        },
    }
