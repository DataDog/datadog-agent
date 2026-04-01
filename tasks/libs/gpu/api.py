from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from datadog_api_client.v2.api.metrics_api import MetricsApi as MetricsApiV2
from datadog_api_client.v2.model.metrics_scalar_query import MetricsScalarQuery

from tasks.libs.gpu.types import GPUConfig

NULLISH_GROUP_VALUES = {"", "none", "null", "n/a"}


@dataclass(slots=True)
class ScalarColumns:
    group: dict[str, list]
    number: dict[str, list]


def normalize_group_value(value: Any) -> str | None:
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


def normalize_device_mode(slicing_mode: str | None, virtualization_mode: str | None) -> str:
    if (slicing_mode or "").lower() == "mig":
        return "mig"
    if (virtualization_mode or "").lower() == "vgpu":
        return "vgpu"
    return "physical"


def _build_scalar_query(name: str, query: str) -> MetricsScalarQuery:
    from datadog_api_client.v2.model.metrics_aggregator import MetricsAggregator
    from datadog_api_client.v2.model.metrics_data_source import MetricsDataSource
    from datadog_api_client.v2.model.metrics_scalar_query import MetricsScalarQuery

    return MetricsScalarQuery(
        name=name,
        aggregator=MetricsAggregator.AVG,
        data_source=MetricsDataSource.METRICS,
        query=query,
    )


def _run_scalar_queries(api: MetricsApiV2, queries: list[Any], from_ts: int, to_ts: int) -> ScalarColumns:
    from datadog_api_client.v2.model.scalar_formula_query_request import ScalarFormulaQueryRequest
    from datadog_api_client.v2.model.scalar_formula_request import ScalarFormulaRequest
    from datadog_api_client.v2.model.scalar_formula_request_attributes import ScalarFormulaRequestAttributes
    from datadog_api_client.v2.model.scalar_formula_request_queries import ScalarFormulaRequestQueries
    from datadog_api_client.v2.model.scalar_formula_request_type import ScalarFormulaRequestType

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
    return _split_scalar_columns(response.data.attributes.columns or [])


def _split_scalar_columns(columns: list[Any]) -> ScalarColumns:
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
    return ScalarColumns(group=group_columns, number=number_columns)


def query_scalar_data(api: MetricsApiV2, query: str, from_ts: int, to_ts: int) -> ScalarColumns:
    return _run_scalar_queries(api, [_build_scalar_query("q0", query)], from_ts, to_ts)


def query_device_count(api: MetricsApiV2, gpu_config: GPUConfig, from_ts: int, to_ts: int) -> int:
    columns = query_scalar_data(
        api,
        f"avg:gpu.device.total{{{gpu_config.to_tag_filter()}}} by {{gpu_uuid}}",
        from_ts,
        to_ts,
    )
    values = columns.number.get("q0", [])
    return sum(1 for value in values if value is not None and value > 0)


def discover_live_gpu_configs(api: MetricsApiV2, from_ts: int, to_ts: int) -> set[tuple[str, str]]:
    columns = query_scalar_data(
        api,
        "avg:gpu.device.total{*} by {gpu_architecture,gpu_slicing_mode,gpu_virtualization_mode}",
        from_ts,
        to_ts,
    )
    values = columns.number.get("q0", [])
    gpu_configs: set[tuple[str, str]] = set()
    for row_idx, value in enumerate(values):
        if value is None or value <= 0:
            continue
        arch_values = columns.group.get("gpu_architecture", [])
        slicing_values = columns.group.get("gpu_slicing_mode", [])
        virtualization_values = columns.group.get("gpu_virtualization_mode", [])
        arch = normalize_group_value(arch_values[row_idx]) if row_idx < len(arch_values) else None
        slicing_mode = normalize_group_value(slicing_values[row_idx]) if row_idx < len(slicing_values) else None
        virtualization_mode = (
            normalize_group_value(virtualization_values[row_idx]) if row_idx < len(virtualization_values) else None
        )
        if not arch:
            continue
        arch = arch.strip().lower()
        if arch in {"n/a", "na", "none", "unknown", ""}:
            continue
        gpu_configs.add((arch, normalize_device_mode(slicing_mode, virtualization_mode)))
    return gpu_configs


def query_expected_metrics_presence_for_gpu_config(
    api: MetricsApiV2,
    metric_names: list[str],
    expected_tags_by_metric: dict[str, set[str]],
    gpu_config_query_filter: str,
    from_ts: int,
    to_ts: int,
) -> tuple[set[str], dict[str, list[str]]]:
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
        queries.append(_build_scalar_query(query_name, query))
        query_name_to_metric[query_name] = metric_name

    columns = _run_scalar_queries(api, queries, from_ts, to_ts)

    present_metrics: set[str] = set()
    tag_failures: dict[str, list[str]] = {}
    for query_name, metric_name in query_name_to_metric.items():
        values = columns.number.get(query_name, [])
        present_row_indexes = [row_idx for row_idx, value in enumerate(values) if value is not None]
        if not present_row_indexes:
            continue
        present_metrics.add(metric_name)
        expected_tags = expected_tags_by_metric.get(metric_name, set())
        if not expected_tags:
            continue
        non_null_seen = {tag: False for tag in expected_tags}
        for row_idx in present_row_indexes:
            for tag in expected_tags:
                tag_values = columns.group.get(tag, [])
                if row_idx >= len(tag_values):
                    continue
                normalized_value = normalize_group_value(tag_values[row_idx])
                if normalized_value is None:
                    continue
                if normalized_value.strip().lower() not in NULLISH_GROUP_VALUES:
                    non_null_seen[tag] = True
        missing_tags = [tag for tag, seen in non_null_seen.items() if not seen]
        if missing_tags:
            tag_failures[metric_name] = missing_tags
    return present_metrics, tag_failures


def list_observed_gpu_metrics_for_gpu_config(
    api: MetricsApiV2, gpu_config: GPUConfig, lookback: int, metric_prefix: str
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
