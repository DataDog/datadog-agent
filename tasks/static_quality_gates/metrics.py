from __future__ import annotations

from dataclasses import dataclass

from tasks.libs.common.datadog_api import query_metrics


@dataclass
class GateMetricsData:
    """Metrics for a single quality gate."""

    current_on_disk_size: int | None = None
    current_on_wire_size: int | None = None
    max_on_disk_size: int | None = None
    max_on_wire_size: int | None = None
    relative_on_disk_size: int | None = None
    relative_on_wire_size: int | None = None


def _extract_gate_name_from_scope(scope: str) -> str | None:
    """Extract gate_name from scope string like 'gate_name:static_quality_gate_agent_deb_amd64'."""
    for part in scope.split(","):
        if part.startswith("gate_name:"):
            return part.split(":", 1)[1]
    return None


def _get_latest_value_from_pointlist(pointlist: list) -> float | None:
    """Get the latest non-null value from a pointlist of Point objects.

    Point.value returns [timestamp, metric_value], so we access index 1.
    """
    if not pointlist:
        return None
    for point in reversed(pointlist):
        if point and point.value and point.value[1] is not None:
            return point.value[1]
    return None


def fetch_pr_metrics(pr_number: int) -> dict[str, GateMetricsData]:
    """
    Fetch metrics for a specific PR from Datadog.

    Uses a single API call to fetch all 4 metric types at once.

    Returns a dict mapping gate_name to GateMetricsData.
    """
    metrics_data: dict[str, GateMetricsData] = {}

    # Map metric names to attribute names
    metric_map = {
        "on_disk_size": "current_on_disk_size",
        "on_wire_size": "current_on_wire_size",
        "max_allowed_on_disk_size": "max_on_disk_size",
        "max_allowed_on_wire_size": "max_on_wire_size",
        "relative_on_disk_size": "relative_on_disk_size",
        "relative_on_wire_size": "relative_on_wire_size",
    }

    # Single query with all metrics (comma-separated)
    queries = ",".join(
        f"avg:datadog.agent.static_quality_gate.{m}{{pr_number:{pr_number}}} by {{gate_name}}" for m in metric_map
    )
    result = query_metrics(queries, from_time="now-1d", to_time="now")

    for series in result:
        gate_name = _extract_gate_name_from_scope(series.get("scope", ""))
        if not gate_name:
            continue

        if gate_name not in metrics_data:
            metrics_data[gate_name] = GateMetricsData()

        expression = series.get("expression", "")
        for metric_suffix, attr_name in metric_map.items():
            if f".{metric_suffix}" in expression:
                latest_value = _get_latest_value_from_pointlist(series.get("pointlist", []))
                if latest_value is not None:
                    setattr(metrics_data[gate_name], attr_name, int(latest_value))
                break

    return metrics_data


def fetch_main_headroom(failing_gates: list[str]) -> dict[str, dict[str, int]]:
    """
    Fetch main branch metrics to calculate headroom (max - current).

    Only fetches metrics for the specified failing gates to minimize API footprint.

    Returns a dict mapping gate_name to {'disk_headroom': int, 'wire_headroom': int}.
    """
    if not failing_gates:
        return {}

    main_metrics: dict[str, dict[str, int]] = {}

    metric_map = {
        "on_disk_size": "current_disk",
        "on_wire_size": "current_wire",
        "max_allowed_on_disk_size": "max_disk",
        "max_allowed_on_wire_size": "max_wire",
    }

    gate_filter = " OR ".join(f"gate_name:{g}" for g in failing_gates)

    queries = ",".join(
        f"avg:datadog.agent.static_quality_gate.{m}{{git_ref:main AND ({gate_filter})}} by {{gate_name}}"
        for m in metric_map
    )
    result = query_metrics(queries, from_time="now-1d", to_time="now")

    for series in result:
        gate_name = _extract_gate_name_from_scope(series.get("scope", ""))
        if not gate_name:
            continue

        if gate_name not in main_metrics:
            main_metrics[gate_name] = {}

        expression = series.get("expression", "")
        for metric_suffix, key in metric_map.items():
            if f".{metric_suffix}" in expression:
                latest_value = _get_latest_value_from_pointlist(series.get("pointlist", []))
                if latest_value is not None:
                    main_metrics[gate_name][key] = int(latest_value)
                break

    headroom: dict[str, dict[str, int]] = {}
    for gate_name, metrics in main_metrics.items():
        disk_headroom = metrics.get("max_disk", 0) - metrics.get("current_disk", 0)
        wire_headroom = metrics.get("max_wire", 0) - metrics.get("current_wire", 0)
        headroom[gate_name] = {
            "disk_headroom": max(0, disk_headroom),
            "wire_headroom": max(0, wire_headroom),
        }

    return headroom
