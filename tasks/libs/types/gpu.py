from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from pydantic import BaseModel, ConfigDict, Field, field_validator

DEVICE_MODES = ("physical", "mig", "vgpu")


class Support(BaseModel):
    model_config = ConfigDict(extra="forbid")
    unsupported_architectures: list[str] = Field(default_factory=list)
    device_features: dict[str, bool | str] = Field(default_factory=dict)
    process_data: bool | str = False

    @field_validator("device_features")
    @classmethod
    def validate_device_features(cls, value: dict[str, bool | str]) -> dict[str, bool | str]:
        allowed_modes = set(DEVICE_MODES)
        invalid_modes = sorted(set(value.keys()) - allowed_modes)
        if invalid_modes:
            raise ValueError(
                f"invalid device modes: {', '.join(invalid_modes)} "
                f"(expected {', '.join(sorted(allowed_modes))})"
            )
        return value


class Metric(BaseModel):
    model_config = ConfigDict(extra="forbid")
    type: str
    tagsets: list[str]
    custom_tags: list[str]
    memory_locations: list[str]
    support: Support = Field(default_factory=Support)
    deprecated: bool = False
    replaced_by: str = ""


class Tagset(BaseModel):
    model_config = ConfigDict(extra="forbid")
    tags: list[str]


class Spec(BaseModel):
    model_config = ConfigDict(extra="forbid")
    metric_prefix: str
    tagsets: dict[str, Tagset]
    metrics: dict[str, Metric]


class Architecture(BaseModel):
    model_config = ConfigDict(extra="ignore")
    unsupported_device_features: list[str] = Field(default_factory=list)


class ArchitecturesSpec(BaseModel):
    model_config = ConfigDict(extra="forbid")
    architectures: dict[str, Architecture]

    def build_combinations(self) -> list["GPUConfig"]:
        combos: list[GPUConfig] = []
        for arch_name, arch in self.architectures.items():
            unsupported = {x.lower() for x in arch.unsupported_device_features}
            for mode in DEVICE_MODES:
                if mode not in unsupported:
                    combos.append(GPUConfig(architecture=arch_name.lower(), device_mode=mode, is_known=True))
        return combos


@dataclass(slots=True)
class GPUConfig:
    architecture: str
    device_mode: str
    is_known: bool = True

    def to_tag_filter(self) -> str:
        parts = [f"gpu_architecture:{self.architecture}"]
        if self.device_mode == "mig":
            parts.append("gpu_slicing_mode:mig")
        elif self.device_mode == "vgpu":
            parts.append("gpu_virtualization_mode:vgpu")
        else:
            parts.append("gpu_virtualization_mode:passthrough")
        return ",".join(parts)

    def to_filter_expression(self) -> str:
        parts = [f"gpu_architecture:{self.architecture}"]
        if self.device_mode == "mig":
            parts.append("gpu_slicing_mode:mig")
        elif self.device_mode == "vgpu":
            parts.append("gpu_virtualization_mode:vgpu")
        else:
            parts.append("gpu_virtualization_mode:passthrough")
        return " AND ".join(parts)

class GPUConfigValidationState(Enum):
    UNKNOWN = "unknown"
    MISSING = "missing"
    FAIL = "fail"
    OK = "ok"

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
        return len(self.missing_metrics) + len(self.unknown_metrics) + len(self.tag_failures) > 0

    def update(self, other: "GPUConfigValidationResult") -> None:
        status_precedence = {
            GPUConfigValidationState.FAIL: 0,
            GPUConfigValidationState.OK: 1,
            GPUConfigValidationState.UNKNOWN: 2,
            GPUConfigValidationState.MISSING: 3,
        }
        other_status = status_precedence[other.state]
        self_status = status_precedence[self.state]
        if other_status < self_status:
            self.state = other.state

        self.present_metrics.update(other.present_metrics)
        self.unknown_metrics.update(other.unknown_metrics)
        self.tag_failures.update(other.tag_failures)

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

    def update(self, other: "ValidationResults") -> None:
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
