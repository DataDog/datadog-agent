#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "datadog-api-client>=2.20.0",
#     "pydantic>=2.0",
#     "pyyaml>=6.0",
# ]
# ///
"""
GPU Metrics Live Validation Script (simplified)

Flow:
1) Build all (architecture, device_mode) combinations from architectures.yaml.
2) Count devices for each combination via gpu.device.total.
3) For each combination, query expected metrics and compute missing metrics.
"""

import argparse
import os
import sys
import time
from pathlib import Path

from datadog_api_client import ApiClient, Configuration
from datadog_api_client.v1.api.metrics_api import MetricsApi
from pydantic import BaseModel, ConfigDict, Field, ValidationError, field_validator  # pyright: ignore[reportMissingImports]
import yaml


DEVICE_MODES = ("physical", "mig", "vgpu")


class SupportModel(BaseModel):
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


class MetricModel(BaseModel):
    model_config = ConfigDict(extra="forbid")
    type: str
    tagsets: list[str]
    custom_tags: list[str]
    memory_locations: list[str]
    support: SupportModel = Field(default_factory=SupportModel)
    deprecated: bool = False
    replaced_by: str = ""


class TagsetModel(BaseModel):
    model_config = ConfigDict(extra="forbid")
    tags: list[str]
    fallback_tags: list[str]


class SpecModel(BaseModel):
    model_config = ConfigDict(extra="forbid")
    metric_prefix: str
    tagsets: dict[str, TagsetModel]
    metrics: dict[str, MetricModel]


class ArchitectureModel(BaseModel):
    model_config = ConfigDict(extra="ignore")
    unsupported_device_features: list[str] = Field(default_factory=list)


class ArchitecturesSpecModel(BaseModel):
    model_config = ConfigDict(extra="forbid")
    architectures: dict[str, ArchitectureModel]


class ComboModel(BaseModel):
    model_config = ConfigDict(extra="forbid")
    architecture: str
    device_mode: str


class ComboValidationResultModel(BaseModel):
    model_config = ConfigDict(extra="forbid")
    combo: ComboModel
    device_count: int
    expected_metrics: set[str]
    present_metrics: set[str]

    @property
    def missing_metrics(self) -> set[str]:
        return self.expected_metrics - self.present_metrics


def load_yaml_model(path: str, model_cls):
    with open(path) as f:
        raw = yaml.safe_load(f)
    try:
        return model_cls.model_validate(raw)
    except ValidationError as e:
        raise ValueError(f"Invalid schema in {path}:\n{e}") from e


def load_spec(spec_path: str) -> SpecModel:
    return load_yaml_model(spec_path, SpecModel)


def load_architectures(architectures_path: str) -> ArchitecturesSpecModel:
    return load_yaml_model(architectures_path, ArchitecturesSpecModel)


def normalize_support_value(value) -> bool | None:
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        lowered = value.strip().lower()
        if lowered == "true":
            return True
        if lowered == "false":
            return False
    return None


def build_combinations(architectures: ArchitecturesSpecModel) -> list[ComboModel]:
    combos: list[ComboModel] = []
    for arch_name, arch in architectures.architectures.items():
        unsupported = {x.lower() for x in arch.unsupported_device_features}
        for mode in DEVICE_MODES:
            if mode not in unsupported:
                combos.append(ComboModel(architecture=arch_name.lower(), device_mode=mode))
    return combos


def get_expected_metrics_for_combo(spec: SpecModel, combo: ComboModel) -> set[str]:
    expected: set[str] = set()
    for metric_name, metric in spec.metrics.items():
        if metric.deprecated:
            continue
        if combo.architecture in metric.support.unsupported_architectures:
            continue

        mode_support = normalize_support_value(metric.support.device_features.get(combo.device_mode))
        if mode_support is False:
            continue

        expected.add(f"{spec.metric_prefix}.{metric_name}")
    return expected


def make_api_client(site: str) -> Configuration:
    config = Configuration()
    config.server_variables["site"] = site
    return config


def combo_filter(combo: ComboModel) -> str:
    parts = [f"gpu_architecture:{combo.architecture}"]
    if combo.device_mode == "mig":
        parts.append("gpu_slicing_mode:mig")
    elif combo.device_mode == "vgpu":
        parts.append("gpu_virtualization_mode:vgpu")
    else:
        # Physical devices are reported as passthrough in current tag schema.
        parts.append("gpu_virtualization_mode:passthrough")
    return ",".join(parts)


def _extract_tag_value(tag_set, key: str) -> str | None:
    for tag in tag_set or []:
        if isinstance(tag, str) and tag.startswith(f"{key}:"):
            return tag.split(":", 1)[1]
    return None


def query_device_count(api: MetricsApi, combo: ComboModel, from_ts: int, to_ts: int) -> int:
    query = f"avg:gpu.device.total{{{combo_filter(combo)}}} by {{gpu_uuid}}"
    response = api.query_metrics(_from=from_ts, to=to_ts, query=query)
    series_list = response.series if hasattr(response, "series") and response.series else []
    uuids = set()
    for series in series_list:
        tag_set = getattr(series, "tag_set", [])
        uuid = _extract_tag_value(tag_set, "gpu_uuid")
        if uuid:
            uuids.add(uuid)
    return len(uuids)


def metric_present_for_combo(
    api: MetricsApi,
    metric_name: str,
    combo_query_filter: str,
    from_ts: int,
    to_ts: int,
) -> bool:
    query = f"avg:{metric_name}{{{combo_query_filter}}}"
    response = api.query_metrics(_from=from_ts, to=to_ts, query=query)
    series_list = response.series if hasattr(response, "series") and response.series else []
    return any(getattr(series, "pointlist", None) for series in series_list)


def validate_combo(
    api: MetricsApi,
    spec: SpecModel,
    combo: ComboModel,
    from_ts: int,
    to_ts: int,
) -> ComboValidationResultModel:
    expected_metrics = get_expected_metrics_for_combo(spec, combo)
    device_count = query_device_count(api, combo, from_ts, to_ts)
    query_filter = combo_filter(combo)

    present_metrics: set[str] = set()
    if device_count > 0:
        total = len(expected_metrics)
        for idx, metric_name in enumerate(sorted(expected_metrics), 1):
            if idx % 20 == 0 or idx == total:
                print(f"    Querying metrics: {idx}/{total}...")
            if metric_present_for_combo(api, metric_name, query_filter, from_ts, to_ts):
                present_metrics.add(metric_name)

    return ComboValidationResultModel(
        combo=combo,
        device_count=device_count,
        expected_metrics=expected_metrics,
        present_metrics=present_metrics,
    )


def main():
    parser = argparse.ArgumentParser(description="Validate GPU metrics by architecture/device-mode combinations")
    parser.add_argument(
        "--spec",
        default=str(Path(__file__).resolve().parents[2] / "spec" / "gpu_metrics.yaml"),
        help="Path to gpu_metrics.yaml",
    )
    parser.add_argument(
        "--architectures",
        default=str(Path(__file__).resolve().parents[2] / "spec" / "architectures.yaml"),
        help="Path to architectures.yaml",
    )
    parser.add_argument("--site", help="Datadog site (overrides DD_SITE env var)")
    parser.add_argument("--lookback-seconds", type=int, default=3600, help="Metrics lookback window")
    args = parser.parse_args()

    spec = load_spec(args.spec)
    architectures = load_architectures(args.architectures)
    combos = build_combinations(architectures)
    site = args.site or os.environ.get("DD_SITE", "datadoghq.com")

    print(f"Loaded metrics spec: {len(spec.metrics)} entries")
    print(f"Loaded architecture spec: {len(architectures.architectures)} architectures")
    print(f"Generated combinations: {len(combos)}")
    print(f"Querying Datadog API at {site}...")

    now = int(time.time())
    from_ts = now - args.lookback_seconds
    failing_count = 0

    config = make_api_client(site)
    with ApiClient(config) as api_client:
        api = MetricsApi(api_client)
        for combo in combos:
            label = f"{combo.architecture}/{combo.device_mode}"
            print(f"\n-- {label} --")
            result = validate_combo(api, spec, combo, from_ts, now)
            print(f"  devices: {result.device_count}")
            print(f"  expected metrics: {len(result.expected_metrics)}")
            if result.device_count == 0:
                print("  present metrics: skipped (no devices)")
                print("  missing metrics: skipped (no devices)")
                continue
            print(f"  present metrics: {len(result.present_metrics)}")
            print(f"  missing metrics: {len(result.missing_metrics)}")
            if result.missing_metrics:
                failing_count += 1
                for name in sorted(result.missing_metrics):
                    print(f"    - {name}")

    print(f"\nCombinations with missing metrics (and devices present): {failing_count}")
    if failing_count > 0:
        sys.exit(1)


if __name__ == "__main__":
    main()
