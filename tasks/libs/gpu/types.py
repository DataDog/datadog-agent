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
class GPUConfigValidationResult:
    config: GPUConfig
    device_count: int
    expected_metrics: set[str]
    state: GPUConfigValidationState = GPUConfigValidationState.UNKNOWN
    present_metrics: set[str] = field(default_factory=set)
    unknown_metrics: set[str] = field(default_factory=set)
    tag_failures: dict[str, list[str]] = field(default_factory=dict)

    @property
    def missing_metrics(self) -> set[str]:
        return self.expected_metrics - self.present_metrics

    @property
    def has_failures(self) -> bool:
        return self.device_count > 0 and (
            len(self.missing_metrics) + len(self.unknown_metrics) + len(self.tag_failures) > 0
        )

    def update(self, other: GPUConfigValidationResult) -> None:
        if other.state.value < self.state.value:
            self.state = other.state

        self.present_metrics.update(other.present_metrics)
        self.unknown_metrics.update(other.unknown_metrics)
        self.tag_failures.update(other.tag_failures)
        self.device_count += other.device_count

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
            expected_metrics=set(item.get("expected_metrics", [])),
            state=STATE_BY_NAME[item["state"]],
            present_metrics=set(item.get("present_metrics", [])),
            unknown_metrics=set(item.get("unknown_metrics", [])),
            tag_failures=dict(item.get("tag_failures", {})),
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
