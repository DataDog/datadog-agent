from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum

from tasks.libs.common.color import color_message
from tasks.static_quality_gates.gates import GateExecutionError, GateMetricHandler, GateResult, byte_to_string

PER_PR_THRESHOLD = 600 * 1024
EXCEPTION_APPROVERS = {"cmourot", "dd-ddamien"}


class GateFailureKind(Enum):
    AbsoluteLimitExceeded = "AbsoluteLimitExceeded"
    PerPRThresholdExceeded = "PerPRThresholdExceeded"
    ExecutionError = "ExecutionError"

    def __str__(self):
        return self.value


@dataclass
class GateVerdict:
    name: str
    failure: GateFailureKind | None
    blocking: bool = False
    message: str | None = None
    blocking_note: str = ""


class ExceptionApprovalChecker:
    """Lazily fetches and caches per-PR threshold exception approval."""

    def __init__(self, pr):
        self._pr = pr
        self._checked = False
        self._result: str | None = None

    def _fetch(self) -> str | None:
        if self._pr is None:
            return None
        try:
            for review in self._pr.get_reviews():
                if review.state == "APPROVED" and review.user and review.user.login in EXCEPTION_APPROVERS:
                    return review.user.login
        except Exception as e:
            print(color_message(f"[WARN] Failed to check exception approvals: {e}", "orange"))
        return None

    def get(self) -> str | None:
        if not self._checked:
            self._checked = True
            self._result = self._fetch()
            if self._result:
                print(color_message(f"Exception granted by @{self._result}", "orange"))
        return self._result


@dataclass
class GateEvaluationResult:
    verdicts: list[GateVerdict] = field(default_factory=list)
    has_blocking_failures: bool = False
    exception_note: str | None = None


def evaluate_gates(
    gate_list,
    gate_results: dict,
    metric_handler: GateMetricHandler,
    *,
    is_on_main_branch: bool,
    is_merge_queue: bool,
    pr,
) -> GateEvaluationResult:
    """Evaluate all gate outcomes and return a structured result."""
    exception_checker = ExceptionApprovalChecker(pr)
    verdicts = [
        evaluate_gate(
            gate_results[gate],
            metric_handler,
            is_on_main_branch=is_on_main_branch,
            is_merge_queue=is_merge_queue,
            exception_checker=exception_checker,
        )
        for gate in gate_list
    ]
    has_blocking_failures = any(v.blocking for v in verdicts)
    exception_granted_by = exception_checker.get()
    exception_note = None
    if exception_granted_by:
        per_pr_excepted = [
            v for v in verdicts if v.failure == GateFailureKind.PerPRThresholdExceeded and not v.blocking
        ]
        if per_pr_excepted:
            threshold_str = byte_to_string(PER_PR_THRESHOLD)
            exception_note = f"**Exception granted by @{exception_granted_by}**: this PR exceeds the per-PR size threshold ({threshold_str}) but will not be blocked.\n"
    return GateEvaluationResult(
        verdicts=verdicts,
        has_blocking_failures=has_blocking_failures,
        exception_note=exception_note,
    )


def evaluate_gate(
    outcome: GateResult | GateExecutionError,
    metric_handler: GateMetricHandler,
    *,
    is_on_main_branch: bool,
    is_merge_queue: bool,
    exception_checker: ExceptionApprovalChecker,
) -> GateVerdict:
    """Evaluate a single gate outcome and return its verdict."""
    if isinstance(outcome, GateExecutionError):
        return GateVerdict(
            name=outcome.name,
            failure=GateFailureKind.ExecutionError,
            blocking=True,
            message=outcome.traceback,
        )

    if not outcome.success:
        print(color_message(outcome.violation_message, "red"))
        # Mark as non-blocking if delta <= 0
        # this tolerance only applies to PRs - on main branch, failures always block unconditionally.
        # A non-positive delta means the size issue existed before this PR and wasn't introduced by the current changes.
        blocking = True
        if not is_on_main_branch and should_bypass_failure(outcome.config.gate_name, metric_handler):
            blocking = False
            print(
                color_message(
                    f"Gate {outcome.config.gate_name} failure is non-blocking (size unchanged from ancestor)", "orange"
                )
            )
        return GateVerdict(
            name=outcome.config.gate_name,
            failure=GateFailureKind.AbsoluteLimitExceeded,
            blocking=blocking,
            blocking_note="" if blocking else "non-blocking: size unchanged from ancestor",
            message=outcome.violation_message,
        )

    # Check per-PR threshold: if a gate increased by more than PER_PR_THRESHOLD, mark it as failing.
    # Skip on merge queue jobs: the MQ working branch is an ephemeral merge preview, not a PR.
    if not is_on_main_branch and not is_merge_queue:
        delta = metric_handler.metrics.get(outcome.config.gate_name, {}).get("relative_on_disk_size", 0)
        if delta > PER_PR_THRESHOLD:
            threshold_str = byte_to_string(PER_PR_THRESHOLD)
            print(color_message(f"Per-PR threshold ({threshold_str}) exceeded by: {outcome.config.gate_name}", "red"))
            approver = exception_checker.get()
            return GateVerdict(
                name=outcome.config.gate_name,
                failure=GateFailureKind.PerPRThresholdExceeded,
                blocking=approver is None,
                blocking_note=f"non-blocking: exception granted by @{approver}" if approver else "",
                message=f"On-disk size increase of {byte_to_string(delta)} exceeds the per-PR threshold of {threshold_str}",
            )

    return GateVerdict(
        name=outcome.config.gate_name,
        failure=None,
        blocking=False,
        message=None,
    )


def should_bypass_failure(gate_name: str, metric_handler: GateMetricHandler) -> bool:
    """
    Check if a gate failure should be non-blocking because on-disk size delta is 0 or negative.

    A failure is considered non-blocking if the on-disk size hasn't increased from the ancestor,
    meaning the issue existed before this PR and wasn't introduced by the current changes.

    Note: Only on-disk size is checked because it's the primary metric for package size impact.

    Args:
        gate_name: The name of the quality gate to check
        metric_handler: The metric handler containing relative size metrics

    Returns:
        True if on-disk size delta is effectively <= 0 (bypass eligible), False otherwise
    """
    gate_metrics = metric_handler.metrics.get(gate_name, {})
    disk_delta = gate_metrics.get("relative_on_disk_size")

    if disk_delta is None:
        return False

    # Threshold: values smaller than 2 KiB are treated as 0
    # Small variations due to build non-determinism should not block PRs
    delta_threshold_bytes = 2 * 1024  # 2 KiB

    return disk_delta <= delta_threshold_bytes
