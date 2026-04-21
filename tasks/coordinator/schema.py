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


@dataclass
class BaselineDetector:
    mean_f1: float
    total_fps: int
    scenarios: dict[str, ScenarioResult]


@dataclass
class Baseline:
    sha: str
    generated_at: str
    detectors: dict[str, BaselineDetector]  # "bocpd", "scanmw", "scanwelch"


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
    api_tokens_used: int
    api_token_ceiling: int | None
    milestones_notified: list[float] = field(default_factory=list)


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

    # DEPRECATED — rolling-reference mechanism dropped (introduced a
    # noise-driven ratchet that let candidates strictly worse than baseline
    # ship). Field kept so old db.yaml files still load; never written.
    # Remove after all active runs migrate.
    last_shipped_per_scenario: dict[str, dict[str, ScenarioResult]] = field(default_factory=dict)


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

    baseline = None
    if d.get("baseline"):
        b = d["baseline"]
        baseline = Baseline(
            sha=b["sha"],
            generated_at=b["generated_at"],
            detectors={
                name: BaselineDetector(
                    mean_f1=v["mean_f1"],
                    total_fps=v["total_fps"],
                    scenarios={s: _scenario(sr) for s, sr in v["scenarios"].items()},
                )
                for name, v in b["detectors"].items()
            },
        )

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
        ),
        iterations=[Iteration(**it) for it in d.get("iterations", [])],
        split=split,
        validations=validations,
        components_eval_dispatched=components_eval_dispatched,
        last_shipped_per_scenario=last_shipped,
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

    baseline_d = None
    if db.baseline:
        baseline_d = {
            "sha": db.baseline.sha,
            "generated_at": db.baseline.generated_at,
            "detectors": {
                name: {
                    "mean_f1": d.mean_f1,
                    "total_fps": d.total_fps,
                    "scenarios": {s: _s(sr) for s, sr in d.scenarios.items()},
                }
                for name, d in db.baseline.detectors.items()
            },
        }

    return {
        "schema_version": db.schema_version,
        "baseline": baseline_d,
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
        "last_shipped_per_scenario": {
            det: {s: _s(sr) for s, sr in scens.items()}
            for det, scens in db.last_shipped_per_scenario.items()
        },
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
