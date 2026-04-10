from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum


class GPUConfigValidationState(Enum):
    FAIL = 0
    OK = 1
    UNKNOWN = 2
    MISSING = 3


STATE_BY_NAME = {
    "fail": GPUConfigValidationState.FAIL,
    "ok": GPUConfigValidationState.OK,
    "unknown": GPUConfigValidationState.UNKNOWN,
    "missing": GPUConfigValidationState.MISSING,
}


@dataclass(slots=True)
class GPUConfig:
    architecture: str
    device_mode: str
    is_known: bool = True


@dataclass(slots=True)
class MetricStatus:
    errors: list[str] = field(default_factory=list)
    tag_errors: dict[str, list[str]] = field(default_factory=dict)


@dataclass(slots=True)
class DetailedValidationResult:
    metrics: dict[str, MetricStatus] = field(default_factory=dict)


@dataclass(slots=True)
class GPUConfigValidationResult:
    config: GPUConfig
    device_count: int
    detailed_result: DetailedValidationResult
    expected_metrics: int
    present_metrics: int
    missing_metrics: int
    unknown_metrics: int
    tag_failures: int
    state: GPUConfigValidationState = GPUConfigValidationState.UNKNOWN

    @property
    def has_failures(self) -> bool:
        return self.device_count > 0 and (self.missing_metrics + self.unknown_metrics + self.tag_failures > 0)

    def update(self, other: GPUConfigValidationResult) -> None:
        if other.state.value < self.state.value:
            self.state = other.state

        self.expected_metrics = max(self.expected_metrics, other.expected_metrics)
        self.present_metrics += other.present_metrics
        self.missing_metrics += other.missing_metrics
        self.unknown_metrics += other.unknown_metrics
        self.tag_failures += other.tag_failures
        self.device_count += other.device_count
        self.detailed_result.metrics.update(other.detailed_result.metrics)

    @property
    def index_key(self) -> tuple[str, str]:
        return (self.config.architecture, self.config.device_mode)


@dataclass
class ValidationResults:
    results: list[GPUConfigValidationResult]
    site: str
    metrics_count: int
    architectures_count: int
    failing_count: int

    def update(self, other: ValidationResults) -> None:
        self.metrics_count = max(self.metrics_count, other.metrics_count)
        self.architectures_count = max(self.architectures_count, other.architectures_count)
        self.failing_count += other.failing_count

        result_index = {result.index_key: result for result in self.results}
        for other_result in other.results:
            if other_result.index_key in result_index:
                result_index[other_result.index_key].update(other_result)
            else:
                result_index[other_result.index_key] = other_result

        self.results = list(result_index.values())


def validation_results_from_dict(payload: dict, *, site: str) -> ValidationResults:
    results = [
        GPUConfigValidationResult(
            config=GPUConfig(
                architecture=item["config"]["architecture"],
                device_mode=item["config"]["device_mode"],
                is_known=item["config"].get("is_known", True),
            ),
            device_count=item["device_count"],
            detailed_result=DetailedValidationResult(
                metrics={
                    metric_name: MetricStatus(
                        errors=list(metric_status.get("errors", [])),
                        tag_errors={tag: list(errors) for tag, errors in (metric_status.get("tag_errors") or {}).items()},
                    )
                    for metric_name, metric_status in ((item.get("detailed_result") or {}).get("metrics") or {}).items()
                }
            ),
            expected_metrics=int(item.get("expected_metrics", 0)),
            present_metrics=int(item.get("present_metrics", 0)),
            missing_metrics=int(item.get("missing_metrics", 0)),
            unknown_metrics=int(item.get("unknown_metrics", 0)),
            tag_failures=int(item.get("tag_failures", 0)),
            state=STATE_BY_NAME[item["state"]],
        )
        for item in payload.get("results", [])
    ]
    return ValidationResults(
        results=results,
        site=site,
        metrics_count=payload["metrics_count"],
        architectures_count=payload["architectures_count"],
        failing_count=payload["failing_count"],
    )
