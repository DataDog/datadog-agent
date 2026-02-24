from __future__ import annotations
# mypy: ignore-errors

import os
import time
from pathlib import Path
from typing import TYPE_CHECKING, Any

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.auth import dd_auth_api_app_keys
from tasks.libs.types.gpu import GPUConfigValidationState

if TYPE_CHECKING:
    from datadog_api_client.v2.api.metrics_api import MetricsApi as MetricsApiV2
    from pydantic import BaseModel
    from tasks.libs.types.gpu import (
        ArchitecturesSpec,
        ValidationResults,
        GPUConfig,
        GPUConfigValidationResult,
        Metric,
        Spec,
    )


def _require_api_keys() -> None:
    if not os.environ.get("DD_API_KEY"):
        raise Exit("DD_API_KEY environment variable is required", code=1)
    if not os.environ.get("DD_APP_KEY"):
        raise Exit("DD_APP_KEY environment variable is required", code=1)


def _resolve_spec_paths(spec: str | None, architectures: str | None) -> tuple[str, str]:
    repo_root = Path(__file__).resolve().parents[1]
    spec_path = spec or str(repo_root / "pkg" / "collector" / "corechecks" / "gpu" / "spec" / "gpu_metrics.yaml")
    architectures_path = architectures or str(
        repo_root / "pkg" / "collector" / "corechecks" / "gpu" / "spec" / "architectures.yaml"
    )
    if not Path(spec_path).exists():
        raise Exit(f"Spec file not found: {spec_path}", code=1)
    if not Path(architectures_path).exists():
        raise Exit(f"Architectures file not found: {architectures_path}", code=1)
    return spec_path, architectures_path


def _color_status(status: GPUConfigValidationState) -> str:
    colors = {
        GPUConfigValidationState.OK: Color.GREEN,
        GPUConfigValidationState.FAIL: Color.RED,
        GPUConfigValidationState.MISSING: Color.ORANGE,
        GPUConfigValidationState.UNKNOWN: Color.ORANGE,
    }
    return color_message(status.value, colors[status]) if status in colors else status.value


def _color_metric_counts(missing: int, known: int, unknown: int) -> str:
    missing_str = color_message(str(missing), Color.RED) if missing > 0 else str(missing)
    known_str = color_message(str(known), Color.GREEN) if known > 0 else str(known)
    unknown_str = color_message(str(unknown), Color.ORANGE) if unknown > 0 else str(unknown)
    return f"{missing_str}/{known_str}/{unknown_str}"


def _color_tag_failures(count: int) -> str:
    return color_message(str(count), Color.RED) if count > 0 else str(count)


def _load_yaml_model(path: str, model_cls: "type[BaseModel]") -> Any:
    from pydantic import ValidationError
    import yaml

    with open(path) as f:
        raw = yaml.safe_load(f)
    try:
        return model_cls.model_validate(raw)
    except ValidationError as e:
        raise ValueError(f"Invalid schema in {path}:\n{e}") from e


def _normalize_support_value(value: Any) -> bool | None:
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        lowered = value.strip().lower()
        if lowered == "true":
            return True
        if lowered == "false":
            return False
    return None


def _get_expected_metrics_for_gpu_config(spec_model: "Spec", gpu_config: "GPUConfig") -> dict[str, "Metric"]:
    expected: dict[str, "Metric"] = {}
    for metric_name, metric in spec_model.metrics.items():
        if metric.deprecated:
            continue
        if gpu_config.architecture in metric.support.unsupported_architectures:
            continue
        mode_support = _normalize_support_value(metric.support.device_features.get(gpu_config.device_mode))
        if not mode_support:
            continue
        expected[f"{spec_model.metric_prefix}.{metric_name}"] = metric
    return expected


def _extract_tag_value(tag_set: Any, key: str) -> str | None:
    for tag in tag_set or []:
        if isinstance(tag, str) and tag.startswith(f"{key}:"):
            return tag.split(":", 1)[1]
    return None


def _query_device_count(api: "MetricsApiV2", gpu_config: "GPUConfig", from_ts: int, to_ts: int) -> int:
    _, number_columns = _query_scalar_data(
        api,
        f"avg:gpu.device.total{{{gpu_config.to_tag_filter()}}} by {{gpu_uuid}}",
        from_ts,
        to_ts,
    )
    values = number_columns.get("q0", [])
    return sum(1 for value in values if value is not None and value > 0)


def _normalize_device_mode(slicing_mode: str | None, virtualization_mode: str | None) -> str:
    if (slicing_mode or "").lower() == "mig":
        return "mig"
    if (virtualization_mode or "").lower() == "vgpu":
        return "vgpu"
    return "physical"


def _discover_live_gpu_configs(api: "MetricsApiV2", from_ts: int, to_ts: int) -> set[tuple[str, str]]:
    group_columns, number_columns = _query_scalar_data(
        api,
        "avg:gpu.device.total{*} by {gpu_architecture,gpu_slicing_mode,gpu_virtualization_mode}",
        from_ts,
        to_ts,
    )
    values = number_columns.get("q0", [])
    gpu_configs: set[tuple[str, str]] = set()
    for row_idx, value in enumerate(values):
        if value is None or value <= 0:
            continue
        arch_values = group_columns.get("gpu_architecture", [])
        slicing_values = group_columns.get("gpu_slicing_mode", [])
        virtualization_values = group_columns.get("gpu_virtualization_mode", [])
        arch = _normalize_group_value(arch_values[row_idx]) if row_idx < len(arch_values) else None
        slicing_mode = _normalize_group_value(slicing_values[row_idx]) if row_idx < len(slicing_values) else None
        virtualization_mode = (
            _normalize_group_value(virtualization_values[row_idx]) if row_idx < len(virtualization_values) else None
        )
        if not arch:
            continue
        arch = arch.strip().lower()
        if arch in {"n/a", "na", "none", "unknown", ""}:
            continue
        gpu_configs.add((arch, _normalize_device_mode(slicing_mode, virtualization_mode)))
    return gpu_configs


def _get_expected_tags_for_metric(spec_model: "Spec", metric: "Metric") -> set[str]:
    tags: set[str] = set()
    for tagset_name in metric.tagsets:
        tagset = spec_model.tagsets.get(tagset_name)
        if tagset:
            tags.update(tagset.tags)
    tags.update(metric.custom_tags)
    return tags


def _chunk(items: list[str], chunk_size: int) -> list[list[str]]:
    return [items[i : i + chunk_size] for i in range(0, len(items), chunk_size)]


def _normalize_group_value(value: Any) -> str | None:
    if value is None:
        return None
    if isinstance(value, list):
        if not value:
            return None
        first = value[0]
        if first is None:
            return None
        return str(first)
    return str(value)


def _query_scalar_data(
    api: "MetricsApiV2",
    query: str,
    from_ts: int,
    to_ts: int,
) -> tuple[dict[str, list], dict[str, list]]:
    from datadog_api_client.v2.model.metrics_aggregator import MetricsAggregator
    from datadog_api_client.v2.model.metrics_data_source import MetricsDataSource
    from datadog_api_client.v2.model.metrics_scalar_query import MetricsScalarQuery
    from datadog_api_client.v2.model.scalar_formula_query_request import ScalarFormulaQueryRequest
    from datadog_api_client.v2.model.scalar_formula_request import ScalarFormulaRequest
    from datadog_api_client.v2.model.scalar_formula_request_attributes import ScalarFormulaRequestAttributes
    from datadog_api_client.v2.model.scalar_formula_request_queries import ScalarFormulaRequestQueries
    from datadog_api_client.v2.model.scalar_formula_request_type import ScalarFormulaRequestType

    scalar_query = MetricsScalarQuery(
        name="q0",
        aggregator=MetricsAggregator.AVG,
        data_source=MetricsDataSource.METRICS,
        query=query,
    )
    body = ScalarFormulaQueryRequest(
        data=ScalarFormulaRequest(
            attributes=ScalarFormulaRequestAttributes(
                _from=from_ts * 1000,
                to=to_ts * 1000,
                queries=ScalarFormulaRequestQueries([scalar_query]),
            ),
            type=ScalarFormulaRequestType.SCALAR_REQUEST,
        )
    )
    response = api.query_scalar_data(body=body)
    columns = response.data.attributes.columns or []
    group_columns: dict[str, list] = {}
    number_columns: dict[str, list] = {}
    for column in columns:
        col_type = str(column.type).lower()
        col_name = column.name
        col_values = column.values or []
        if col_type == "group":
            group_columns[col_name] = col_values
        elif col_type == "number":
            number_columns[col_name] = col_values
    return group_columns, number_columns


def _query_expected_metrics_presence_for_gpu_config(
    api: "MetricsApiV2",
    metric_names: list[str],
    expected_tags_by_metric: dict[str, set[str]],
    gpu_config_query_filter: str,
    from_ts: int,
    to_ts: int,
) -> tuple[set[str], dict[str, list[str]]]:
    from datadog_api_client.v2.model.metrics_aggregator import MetricsAggregator
    from datadog_api_client.v2.model.metrics_data_source import MetricsDataSource
    from datadog_api_client.v2.model.metrics_scalar_query import MetricsScalarQuery
    from datadog_api_client.v2.model.scalar_formula_query_request import ScalarFormulaQueryRequest
    from datadog_api_client.v2.model.scalar_formula_request import ScalarFormulaRequest
    from datadog_api_client.v2.model.scalar_formula_request_attributes import ScalarFormulaRequestAttributes
    from datadog_api_client.v2.model.scalar_formula_request_queries import ScalarFormulaRequestQueries
    from datadog_api_client.v2.model.scalar_formula_request_type import ScalarFormulaRequestType

    if not metric_names:
        return set(), {}

    queries: list[Any] = []
    query_name_to_metric: dict[str, str] = {}
    for idx, metric_name in enumerate(metric_names):
        query_name = f"q{idx}"
        query = f"avg:{metric_name}{{{gpu_config_query_filter}}}"
        expected_tags = expected_tags_by_metric.get(metric_name, set())
        if expected_tags:
            query = f"{query} by {{{','.join(expected_tags)}}}"
        queries.append(
            MetricsScalarQuery(
                name=query_name,
                aggregator=MetricsAggregator.AVG,
                data_source=MetricsDataSource.METRICS,
                query=query,
            )
        )
        query_name_to_metric[query_name] = metric_name

    body = ScalarFormulaQueryRequest(
        data=ScalarFormulaRequest(
            attributes=ScalarFormulaRequestAttributes(
                _from=from_ts * 1000,
                to=to_ts * 1000,
                queries=ScalarFormulaRequestQueries(queries),
            ),
            type=ScalarFormulaRequestType.SCALAR_REQUEST,
        )
    )
    response = api.query_scalar_data(body=body)
    columns = response.data.attributes.columns or []

    group_columns: dict[str, list] = {}
    number_columns: dict[str, list] = {}
    for column in columns:
        col_type = str(column.type).lower()
        col_name = column.name
        col_values = column.values or []
        if col_type == "group":
            group_columns[col_name] = col_values
        elif col_type == "number":
            number_columns[col_name] = col_values

    present_metrics: set[str] = set()
    tag_failures: dict[str, list[str]] = {}
    nullish_values = {"", "none", "null", "n/a"}
    for query_name, metric_name in query_name_to_metric.items():
        values = number_columns.get(query_name, [])
        present_row_indexes = [row_idx for row_idx, value in enumerate(values) if value is not None]
        if not present_row_indexes:
            continue
        present_metrics.add(metric_name)
        expected_tags = expected_tags_by_metric.get(metric_name, [])
        if not expected_tags:
            continue
        non_null_seen = {tag: False for tag in expected_tags}
        for row_idx in present_row_indexes:
            for tag in expected_tags:
                tag_values = group_columns.get(tag, [])
                if row_idx >= len(tag_values):
                    continue
                normalized_value = _normalize_group_value(tag_values[row_idx])
                if normalized_value is None:
                    continue
                if normalized_value.strip().lower() not in nullish_values:
                    non_null_seen[tag] = True
        missing_tags = [tag for tag, seen in non_null_seen.items() if not seen]
        if missing_tags:
            tag_failures[metric_name] = missing_tags
    return present_metrics, tag_failures


def _list_observed_gpu_metrics_for_gpu_config(
    api: "MetricsApiV2", gpu_config: "GPUConfig", lookback: int, metric_prefix: str
) -> set[str]:
    metrics: set[str] = set()
    filter_expr = gpu_config.to_filter_expression()
    page_cursor = None
    while True:
        kwargs = {
            "filter_tags": filter_expr,
            "filter_queried": True,
            "window_seconds": max(lookback, 3600),
            "page_size": 1000,
        }
        if page_cursor:
            kwargs["page_cursor"] = page_cursor
        response = api.list_tag_configurations(**kwargs)
        for item in response.data or []:
            metric_name = item.id
            if metric_name.startswith(f"{metric_prefix}."):
                metrics.add(metric_name)
        page_cursor = response.meta.pagination.next_cursor if response.meta and response.meta.pagination else None
        if not page_cursor:
            break
    return metrics


def _validate_gpu_config(
    metrics_api_v2: "MetricsApiV2",
    spec_model: "Spec",
    gpu_config: "GPUConfig",
    from_ts: int,
    to_ts: int,
    scalar_query_batch_size: int,
) -> "GPUConfigValidationResult":
    from tasks.libs.types.gpu import GPUConfigValidationResult

    expected_metrics_map = _get_expected_metrics_for_gpu_config(spec_model, gpu_config)
    expected_metrics = list(expected_metrics_map.keys())
    device_count = _query_device_count(metrics_api_v2, gpu_config, from_ts, to_ts)
    query_filter = gpu_config.to_tag_filter()

    result = GPUConfigValidationResult(
        config=gpu_config,
        device_count=device_count,
        expected_metrics=set(expected_metrics),
    )

    if device_count == 0:
        result.state = GPUConfigValidationState.MISSING
        return result

    expected_tags_by_metric: dict[str, set[str]] = {}
    for metric_name in expected_metrics_map.keys():
        relative_name = metric_name.removeprefix(f"{spec_model.metric_prefix}.")
        expected_tags_by_metric[metric_name] = _get_expected_tags_for_metric(spec_model, spec_model.metrics[relative_name])

    for metric_batch in _chunk(expected_metrics, scalar_query_batch_size):
        batch_present, batch_failures = _query_expected_metrics_presence_for_gpu_config(
            metrics_api_v2,
            metric_batch,
            expected_tags_by_metric,
            query_filter,
            from_ts,
            to_ts,
        )
        result.present_metrics.update(batch_present)
        result.tag_failures.update(batch_failures)

    live_gpu_metrics = _list_observed_gpu_metrics_for_gpu_config(
        metrics_api_v2,
        gpu_config,
        max(to_ts - from_ts, 0),
        spec_model.metric_prefix,
    )
    result.unknown_metrics = live_gpu_metrics - set(expected_metrics)

    if not gpu_config.is_known:
        result.state = GPUConfigValidationState.UNKNOWN
    elif result.missing_metrics or result.unknown_metrics or result.tag_failures:
        result.state = GPUConfigValidationState.FAIL
    else:
        result.state = GPUConfigValidationState.OK

    return result


def _combine_known_and_live_gpu_configs(
    known_gpu_configs: list["GPUConfig"],
    live_gpu_config_keys: set[tuple[str, str]],
) -> list["GPUConfig"]:
    from tasks.libs.types.gpu import GPUConfig

    by_key: dict[tuple[str, str], "GPUConfig"] = {(c.architecture, c.device_mode): c for c in known_gpu_configs}
    for key in sorted(live_gpu_config_keys):
        if key not in by_key:
            by_key[key] = GPUConfig(architecture=key[0], device_mode=key[1], is_known=False)
    return sorted(by_key.values(), key=lambda gpu_config: (gpu_config.architecture, gpu_config.device_mode))


def _compute_validation(
    spec_path: str,
    architectures_path: str,
    site: str,
    lookback_seconds: int,
) -> "ValidationResults":
    from datadog_api_client import ApiClient, Configuration
    from datadog_api_client.v2.api.metrics_api import MetricsApi as MetricsApiV2
    from tasks.libs.types.gpu import ArchitecturesSpec, Spec
    from tasks.libs.types.gpu import ValidationResults

    scalar_query_batch_size = 50

    spec_model = _load_yaml_model(spec_path, Spec)
    architectures_model = _load_yaml_model(architectures_path, ArchitecturesSpec)
    now = int(time.time())
    from_ts = now - int(lookback_seconds)
    known_gpu_configs = architectures_model.build_combinations()
    failing_count = 0

    config = Configuration()
    config.server_variables["site"] = site
    results: list[GPUConfigValidationResult] = []
    with ApiClient(config) as api_client:
        metrics_api_v2 = MetricsApiV2(api_client)
        live_gpu_config_keys = _discover_live_gpu_configs(metrics_api_v2, from_ts, now)
        gpu_configs = _combine_known_and_live_gpu_configs(known_gpu_configs, live_gpu_config_keys)

        print(f"Validating {len(gpu_configs)} GPU configs...")

        for gpu_config in gpu_configs:
            result = _validate_gpu_config(metrics_api_v2, spec_model, gpu_config, from_ts, now, scalar_query_batch_size)

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


def _print_summary_table(title: str, results: list["GPUConfigValidationResult"]) -> None:
    from tabulate import tabulate

    rows = [
        [
            row.config.architecture,
            row.config.device_mode,
            _color_status(row.state),
            row.device_count,
            _color_metric_counts(len(row.missing_metrics), len(row.present_metrics), len(row.unknown_metrics)),
            _color_tag_failures(len(row.tag_failures)),
        ]
        for row in results
    ]

    print(f"\n{title}:")
    print(
        tabulate(
            rows,
            headers=[
                "architecture",
                "device mode",
                "status",
                "found devices",
                "missing/known/unknown metrics",
                "tag failures",
            ],
            tablefmt="github",
        )
    )


def _print_result_details(results: list["GPUConfigValidationResult"]) -> None:
    print("\nValidation details (showing only failures on configs with devices present):")
    for result in results:
        if not result.has_failures or result.device_count == 0:
            continue
        print(f"\n-- {result.config.architecture} {result.config.device_mode} --")
        print(f"  found devices: {result.device_count}")
        if result.missing_metrics:
            print("  missing metric names:")
            for name in result.missing_metrics:
                print(f"    - MISSING {name}")
        if result.unknown_metrics:
            print("  unknown metric names:")
            for name in result.unknown_metrics:
                print(f"    - UNKNOWN {name}")
        if result.tag_failures:
            print("  tag failure details:")
            for metric_name, tags in result.tag_failures.items():
                print(f"    - TAG FAIL {metric_name}: missing/non-null [{', '.join(tags)}]")


def _render_results(result: "ValidationResults") -> None:
    print(f"Loaded metrics spec: {result.metrics_count} entries")
    print(f"Loaded architecture spec: {result.architectures_count} architectures")
    print(f"Target site: {result.site}")
    _print_summary_table("Summary", result.results)
    _print_result_details(result.results)
    print(f"\nTotal combinations with metric/tag failures (and devices present): {result.failing_count}")


@task(
    name="validate-metrics-single-org",
    help={
        "spec": "Path to gpu_metrics.yaml",
        "architectures": "Path to architectures.yaml",
        "site": "Datadog site (defaults to datadoghq.com)",
        "lookback_seconds": "Metrics lookback window in seconds",
    },
)
def validate_metrics_single_org(_, spec=None, architectures=None, site="datadoghq.com", lookback_seconds=3600):
    """
    Validate live GPU metrics by architecture/device-mode combinations for one org.
    """
    _require_api_keys()
    spec_path, architectures_path = _resolve_spec_paths(spec, architectures)
    result = _compute_validation(spec_path, architectures_path, site, int(lookback_seconds))
    _render_results(result)
    if result.failing_count > 0:
        raise Exit(code=1)


@task(
    name="validate-metrics-all-dd",
    help={
        "spec": "Path to gpu_metrics.yaml",
        "architectures": "Path to architectures.yaml",
        "lookback_seconds": "Metrics lookback window in seconds",
    },
)
def validate_metrics_all_dd(ctx, spec=None, architectures=None, lookback_seconds=3600):
    """
    Validate live GPU metrics for Datadog prod and staging using dd-auth credentials.
    """
    spec_path, architectures_path = _resolve_spec_paths(spec, architectures)
    orgs = [("prod", "app.datadoghq.com"), ("staging", "ddstaging.datadoghq.com")]

    results: ValidationResults | None = None
    org_errors: list[str] = []
    for org_name, dd_auth_domain in orgs:
        print(f"\n== Running GPU validation for {org_name} ({dd_auth_domain}) ==")
        try:
            with dd_auth_api_app_keys(ctx, dd_auth_domain):
                _require_api_keys()
                result = _compute_validation(spec_path, architectures_path, "datadoghq.com", int(lookback_seconds))
                if results is None:
                    results = result
                else:
                    results.update(result)
        except Exception as e:
            org_errors.append(f"{org_name}: {e}")
            print(f"[ERROR] {org_name} failed: {e}")


    if results:
        _render_results(results)

    if org_errors:
        print("\nOrg execution errors:")
        for err in org_errors:
            print(f"  - {err}")
        raise Exit(code=1)

    if results and results.failing_count > 0:
        raise Exit(code=1)
