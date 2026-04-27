from __future__ import annotations

from tasks.libs.common.color import color_message
from tasks.static_quality_gates.gates import GateMetricHandler

PER_PR_THRESHOLD = 600 * 1024
EXCEPTION_APPROVERS = {"cmourot", "dd-ddamien"}


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


def identify_gates_exceeding_pr_threshold(metric_handler: GateMetricHandler) -> list[str]:
    """
    Identify gates where the on-disk size increase exceeds PER_PR_THRESHOLD.

    Returns gate names where relative_on_disk_size > PER_PR_THRESHOLD.
    """
    exceeding = []
    for gate_name, gate_metrics in metric_handler.metrics.items():
        delta = gate_metrics.get("relative_on_disk_size")
        if delta is not None and delta > PER_PR_THRESHOLD:
            exceeding.append(gate_name)
    return exceeding


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
