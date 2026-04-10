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


@dataclass(slots=True)
class TagSummary:
    found: int = 0
    missing: int = 0
    unknown: int = 0
    invalid_value: int = 0

    @property
    def has_failures(self) -> bool:
        return self.missing > 0 or self.unknown > 0 or self.invalid_value > 0


@dataclass(slots=True)
class MetricStatus:
    errors: list[str] = field(default_factory=list)
    tag_results: dict[str, TagSummary] = field(default_factory=dict)

    @property
    def has_failures(self) -> bool:
        return bool(self.errors) or any(tag_result.has_failures for tag_result in self.tag_results.values())

    def update(self, other: MetricStatus) -> None:
        self.errors.extend(other.errors)
        for tag_name, other_tag_result in other.tag_results.items():
            if tag_name not in self.tag_results:
                self.tag_results[tag_name] = TagSummary()
            current = self.tag_results[tag_name]
            current.found += other_tag_result.found
            current.missing += other_tag_result.missing
            current.unknown += other_tag_result.unknown
            current.invalid_value += other_tag_result.invalid_value


@dataclass(slots=True)
class DetailedValidationResult:
    metrics: dict[str, MetricStatus] = field(default_factory=dict)

    def update(self, other: DetailedValidationResult) -> None:
        for metric_name, other_metric_status in other.metrics.items():
            if metric_name not in self.metrics:
                self.metrics[metric_name] = MetricStatus()
            self.metrics[metric_name].update(other_metric_status)


@dataclass(slots=True)
class GPUConfigValidationResult:
    config: GPUConfig
    device_count: int
    detailed_result: DetailedValidationResult
    state: GPUConfigValidationState = GPUConfigValidationState.UNKNOWN

    def update(self, other: GPUConfigValidationResult) -> None:
        if other.state.value < self.state.value:
            self.state = other.state

        self.device_count += other.device_count
        self.detailed_result.update(other.detailed_result)

    @property
    def index_key(self) -> tuple[str, str]:
        return (self.config.architecture, self.config.device_mode)

    @property
    def missing_metrics(self) -> int:
        return sum(1 for metric_status in self.detailed_result.metrics.values() if "missing" in metric_status.errors)

    @property
    def unknown_metrics(self) -> int:
        return sum(
            1
            for metric_status in self.detailed_result.metrics.values()
            if "unknown" in metric_status.errors or "unsupported" in metric_status.errors
        )

    @property
    def present_metrics(self) -> int:
        return len(self.detailed_result.metrics) - self.missing_metrics

    @property
    def tag_failures(self) -> int:
        return sum(1 for metric_status in self.detailed_result.metrics.values() if metric_status.has_failures and metric_status.tag_results)


@dataclass
class ValidationResults:
    results: list[GPUConfigValidationResult]
    site: str
    metrics_count: int
    architectures_count: int

    def update(self, other: ValidationResults) -> None:
        self.metrics_count = max(self.metrics_count, other.metrics_count)
        self.architectures_count = max(self.architectures_count, other.architectures_count)

        result_index = {result.index_key: result for result in self.results}
        for other_result in other.results:
            if other_result.index_key in result_index:
                result_index[other_result.index_key].update(other_result)
            else:
                result_index[other_result.index_key] = other_result

        self.results = list(result_index.values())

    @property
    def failing_count(self) -> int:
        return sum(1 for result in self.results if result.device_count > 0 and result.state is GPUConfigValidationState.FAIL)


def validation_results_from_dict(payload: dict, *, site: str) -> ValidationResults:
    results = [
        GPUConfigValidationResult(
            config=GPUConfig(
                architecture=item["config"]["architecture"],
                device_mode=item["config"]["device_mode"],
            ),
            device_count=item["device_count"],
            detailed_result=DetailedValidationResult(
                metrics={
                    metric_name: MetricStatus(
                        errors=list(metric_status.get("errors", [])),
                        tag_results={
                            tag: TagSummary(
                                found=int(tag_result.get("found", 0)),
                                missing=int(tag_result.get("missing", 0)),
                                unknown=int(tag_result.get("unknown", 0)),
                                invalid_value=int(tag_result.get("invalid_value", 0)),
                            )
                            for tag, tag_result in (metric_status.get("tag_results") or {}).items()
                        },
                    )
                    for metric_name, metric_status in ((item.get("detailed_result") or {}).get("metrics") or {}).items()
                }
            ),
            state=STATE_BY_NAME[item["state"]],
        )
        for item in payload.get("results", [])
    ]
    return ValidationResults(
        results=results,
        site=site,
        metrics_count=payload["metrics_count"],
        architectures_count=payload["architectures_count"],
    )
