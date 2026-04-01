from __future__ import annotations

import os
import time
from pathlib import Path
from typing import TYPE_CHECKING, Any, TypeVar

from invoke.exceptions import Exit

from tasks.libs.gpu.api import (
    discover_live_gpu_configs,
    list_observed_gpu_metrics_for_gpu_config,
    query_device_count,
    query_expected_metrics_presence_for_gpu_config,
)
from tasks.libs.gpu.types import (
    ArchitecturesSpec,
    GPUConfig,
    GPUConfigValidationResult,
    GPUConfigValidationState,
    Metric,
    Spec,
    ValidationResults,
)

if TYPE_CHECKING:
    from datadog_api_client.v2.api.metrics_api import MetricsApi as MetricsApiV2
    from pydantic import BaseModel


SCALAR_QUERY_BATCH_SIZE = 50


def require_api_keys() -> None:
    if not os.environ.get("DD_API_KEY"):
        raise Exit("DD_API_KEY environment variable is required", code=1)
    if not os.environ.get("DD_APP_KEY"):
        raise Exit("DD_APP_KEY environment variable is required", code=1)


def resolve_spec_paths(spec: str | None, architectures: str | None) -> tuple[str, str]:
    repo_root = Path(__file__).resolve().parents[3]
    spec_path = spec or str(repo_root / "pkg" / "collector" / "corechecks" / "gpu" / "spec" / "gpu_metrics.yaml")
    architectures_path = architectures or str(
        repo_root / "pkg" / "collector" / "corechecks" / "gpu" / "spec" / "architectures.yaml"
    )
    if not Path(spec_path).exists():
        raise Exit(f"Spec file not found: {spec_path}", code=1)
    if not Path(architectures_path).exists():
        raise Exit(f"Architectures file not found: {architectures_path}", code=1)
    return spec_path, architectures_path


ModelT = TypeVar("ModelT", bound=BaseModel)


def load_yaml_model(path: str, model_cls: type[ModelT]) -> ModelT:
    import yaml
    from pydantic import ValidationError

    with open(path) as f:
        raw = yaml.safe_load(f)
    try:
        return model_cls.model_validate(raw)
    except ValidationError as e:
        raise ValueError(f"Invalid schema in {path}:\n{e}") from e


def get_expected_metrics_for_gpu_config(spec_model: Spec, gpu_config: GPUConfig) -> dict[str, Metric]:
    expected: dict[str, Metric] = {}
    for metric_name, metric in spec_model.metrics.items():
        if metric.deprecated:
            continue
        if gpu_config.architecture in metric.support.unsupported_architectures:
            continue
        mode_support = metric.support.device_modes.get(gpu_config.device_mode)
        if not mode_support:
            continue
        expected[f"{spec_model.metric_prefix}.{metric_name}"] = metric
    return expected


def get_expected_tags_for_metric(spec_model: Spec, metric: Metric) -> set[str]:
    tags: set[str] = set()
    for tagset_name in metric.tagsets:
        tagset = spec_model.tagsets.get(tagset_name)
        if tagset:
            tags.update(tagset.tags)
    tags.update(metric.custom_tags)
    return tags


def batch_items(items: list[str], chunk_size: int) -> list[list[str]]:
    return [items[i : i + chunk_size] for i in range(0, len(items), chunk_size)]


def _build_expected_tags_by_metric(spec_model: Spec, expected_metrics_map: dict[str, Metric]) -> dict[str, set[str]]:
    expected_tags_by_metric: dict[str, set[str]] = {}
    for metric_name in expected_metrics_map:
        relative_name = metric_name.removeprefix(f"{spec_model.metric_prefix}.")
        expected_tags_by_metric[metric_name] = get_expected_tags_for_metric(
            spec_model, spec_model.metrics[relative_name]
        )
    return expected_tags_by_metric


def determine_result_state(result: GPUConfigValidationResult) -> GPUConfigValidationState:
    if not result.config.is_known:
        return GPUConfigValidationState.UNKNOWN
    if result.missing_metrics or result.unknown_metrics or result.tag_failures:
        return GPUConfigValidationState.FAIL
    return GPUConfigValidationState.OK


def validate_gpu_config(
    metrics_api_v2: MetricsApiV2,
    spec_model: Spec,
    gpu_config: GPUConfig,
    from_ts: int,
    to_ts: int,
    scalar_query_batch_size: int = SCALAR_QUERY_BATCH_SIZE,
) -> GPUConfigValidationResult:
    expected_metrics_map = get_expected_metrics_for_gpu_config(spec_model, gpu_config)
    expected_metrics = list(expected_metrics_map.keys())
    device_count = query_device_count(metrics_api_v2, gpu_config, from_ts, to_ts)
    query_filter = gpu_config.to_tag_filter()

    result = GPUConfigValidationResult(
        config=gpu_config,
        device_count=device_count,
        expected_metrics=set(expected_metrics),
    )

    if device_count == 0:
        result.state = GPUConfigValidationState.MISSING
        return result

    expected_tags_by_metric = _build_expected_tags_by_metric(spec_model, expected_metrics_map)

    for metric_batch in batch_items(expected_metrics, scalar_query_batch_size):
        batch_present, batch_failures = query_expected_metrics_presence_for_gpu_config(
            metrics_api_v2,
            metric_batch,
            expected_tags_by_metric,
            query_filter,
            from_ts,
            to_ts,
        )
        result.present_metrics.update(batch_present)
        result.tag_failures.update(batch_failures)

    live_gpu_metrics = list_observed_gpu_metrics_for_gpu_config(
        metrics_api_v2,
        gpu_config,
        max(to_ts - from_ts, 0),
        spec_model.metric_prefix,
    )
    result.unknown_metrics = live_gpu_metrics - set(expected_metrics)
    result.state = determine_result_state(result)
    return result


def combine_known_and_live_gpu_configs(
    known_gpu_configs: list[GPUConfig],
    live_gpu_config_keys: set[tuple[str, str]],
) -> list[GPUConfig]:
    by_key: dict[tuple[str, str], GPUConfig] = {(c.architecture, c.device_mode): c for c in known_gpu_configs}
    for key in sorted(live_gpu_config_keys):
        if key not in by_key:
            by_key[key] = GPUConfig(architecture=key[0], device_mode=key[1], is_known=False)
    return sorted(by_key.values(), key=lambda gpu_config: (gpu_config.architecture, gpu_config.device_mode))


def compute_validation(
    spec_path: str,
    architectures_path: str,
    site: str,
    lookback_seconds: int,
    progress_writer: Any | None = None,
) -> ValidationResults:
    from datadog_api_client import ApiClient, Configuration
    from datadog_api_client.v2.api.metrics_api import MetricsApi as MetricsApiV2

    spec_model = load_yaml_model(spec_path, Spec)
    architectures_model = load_yaml_model(architectures_path, ArchitecturesSpec)
    now = int(time.time())
    from_ts = now - int(lookback_seconds)
    known_gpu_configs = architectures_model.build_combinations()
    failing_count = 0

    config = Configuration()
    config.server_variables["site"] = site
    results: list[GPUConfigValidationResult] = []
    with ApiClient(config) as api_client:
        metrics_api_v2 = MetricsApiV2(api_client)
        live_gpu_config_keys = discover_live_gpu_configs(metrics_api_v2, from_ts, now)
        gpu_configs = combine_known_and_live_gpu_configs(known_gpu_configs, live_gpu_config_keys)

        if progress_writer is not None:
            progress_writer(f"Validating {len(gpu_configs)} GPU configs...")

        for gpu_config in gpu_configs:
            result = validate_gpu_config(metrics_api_v2, spec_model, gpu_config, from_ts, now, SCALAR_QUERY_BATCH_SIZE)

            if not gpu_config.is_known and result.device_count == 0:
                continue

            results.append(result)

            if gpu_config.is_known and result.has_failures:
                failing_count += 1

    return ValidationResults(
        site=site,
        metrics_count=len(spec_model.metrics),
        architectures_count=len(architectures_model.architectures),
        results=results,
        failing_count=failing_count,
    )
