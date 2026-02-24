from __future__ import annotations
# mypy: ignore-errors

import os
import time
from pathlib import Path
from typing import Any

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.auth import dd_auth_api_app_keys


def _load_gpu_validation_deps():
    try:
        from datadog_api_client import ApiClient, Configuration
        from datadog_api_client.v1.api.metrics_api import MetricsApi as MetricsApiV1
        from datadog_api_client.v2.api.metrics_api import MetricsApi as MetricsApiV2
        from datadog_api_client.v2.model.metrics_aggregator import MetricsAggregator
        from datadog_api_client.v2.model.metrics_data_source import MetricsDataSource
        from datadog_api_client.v2.model.metrics_scalar_query import MetricsScalarQuery
        from datadog_api_client.v2.model.scalar_formula_query_request import ScalarFormulaQueryRequest
        from datadog_api_client.v2.model.scalar_formula_request import ScalarFormulaRequest
        from datadog_api_client.v2.model.scalar_formula_request_attributes import ScalarFormulaRequestAttributes
        from datadog_api_client.v2.model.scalar_formula_request_queries import ScalarFormulaRequestQueries
        from datadog_api_client.v2.model.scalar_formula_request_type import ScalarFormulaRequestType
        from pydantic import BaseModel, ConfigDict, Field, ValidationError, field_validator
        from tabulate import tabulate
        import yaml
    except ImportError as e:
        raise Exit(
            "Missing Python dependencies for gpu validation task.\n"
            "Install with:\n"
            "  dda inv --dep \"datadog-api-client>=2.20.0\" --dep \"pydantic>=2.0\" --dep \"pyyaml>=6.0\" --dep \"tabulate>=0.9.0\" gpu.validate-metrics-single-org",
            code=1,
        ) from e

    return {
        "ApiClient": ApiClient,
        "Configuration": Configuration,
        "MetricsApiV1": MetricsApiV1,
        "MetricsApiV2": MetricsApiV2,
        "MetricsAggregator": MetricsAggregator,
        "MetricsDataSource": MetricsDataSource,
        "MetricsScalarQuery": MetricsScalarQuery,
        "ScalarFormulaQueryRequest": ScalarFormulaQueryRequest,
        "ScalarFormulaRequest": ScalarFormulaRequest,
        "ScalarFormulaRequestAttributes": ScalarFormulaRequestAttributes,
        "ScalarFormulaRequestQueries": ScalarFormulaRequestQueries,
        "ScalarFormulaRequestType": ScalarFormulaRequestType,
        "BaseModel": BaseModel,
        "ConfigDict": ConfigDict,
        "Field": Field,
        "ValidationError": ValidationError,
        "field_validator": field_validator,
        "tabulate": tabulate,
        "yaml": yaml,
    }


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


def _color_status(status: str) -> str:
    colors = {"ok": "\033[32m", "fail": "\033[31m", "missing": "\033[33m", "unknown": "\033[33m"}
    reset = "\033[0m"
    return f"{colors.get(status, '')}{status}{reset}" if status in colors else status


def _color_metric_counts(missing: int, known: int, unknown: int) -> str:
    red = "\033[31m"
    green = "\033[32m"
    yellow = "\033[33m"
    reset = "\033[0m"
    missing_str = f"{red}{missing}{reset}" if missing > 0 else str(missing)
    known_str = f"{green}{known}{reset}" if known > 0 else str(known)
    unknown_str = f"{yellow}{unknown}{reset}" if unknown > 0 else str(unknown)
    return f"{missing_str}/{known_str}/{unknown_str}"


def _color_tag_failures(count: int) -> str:
    red = "\033[31m"
    reset = "\033[0m"
    return f"{red}{count}{reset}" if count > 0 else str(count)


def _compute_validation(
    deps: dict[str, Any],
    spec_path: str,
    architectures_path: str,
    site: str,
    lookback_seconds: int,
) -> dict[str, Any]:
    ApiClient = deps["ApiClient"]
    Configuration = deps["Configuration"]
    MetricsApiV1 = deps["MetricsApiV1"]
    MetricsApiV2 = deps["MetricsApiV2"]
    MetricsAggregator = deps["MetricsAggregator"]
    MetricsDataSource = deps["MetricsDataSource"]
    MetricsScalarQuery = deps["MetricsScalarQuery"]
    ScalarFormulaQueryRequest = deps["ScalarFormulaQueryRequest"]
    ScalarFormulaRequest = deps["ScalarFormulaRequest"]
    ScalarFormulaRequestAttributes = deps["ScalarFormulaRequestAttributes"]
    ScalarFormulaRequestQueries = deps["ScalarFormulaRequestQueries"]
    ScalarFormulaRequestType = deps["ScalarFormulaRequestType"]
    BaseModel = deps["BaseModel"]
    ConfigDict = deps["ConfigDict"]
    Field = deps["Field"]
    ValidationError = deps["ValidationError"]
    field_validator = deps["field_validator"]
    yaml = deps["yaml"]

    device_modes = ("physical", "mig", "vgpu")
    scalar_query_batch_size = 50

    class SupportModel(BaseModel):
        model_config = ConfigDict(extra="forbid")
        unsupported_architectures: list[str] = Field(default_factory=list)
        device_features: dict[str, bool | str] = Field(default_factory=dict)
        process_data: bool | str = False

        @field_validator("device_features")
        @classmethod
        def validate_device_features(cls, value: dict[str, bool | str]) -> dict[str, bool | str]:
            allowed_modes = set(device_modes)
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
        is_known: bool = True

    class ComboValidationResultModel(BaseModel):
        model_config = ConfigDict(extra="forbid")
        combo: ComboModel
        device_count: int
        expected_metrics: set[str]
        present_metrics: set[str]
        unknown_metrics: set[str] = Field(default_factory=set)
        tag_failures: dict[str, list[str]] = Field(default_factory=dict)

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

    def build_combinations(architectures_spec: ArchitecturesSpecModel) -> list[ComboModel]:
        combos: list[ComboModel] = []
        for arch_name, arch in architectures_spec.architectures.items():
            unsupported = {x.lower() for x in arch.unsupported_device_features}
            for mode in device_modes:
                if mode not in unsupported:
                    combos.append(ComboModel(architecture=arch_name.lower(), device_mode=mode, is_known=True))
        return combos

    def get_expected_metrics_for_combo(spec_model: SpecModel, combo: ComboModel) -> dict[str, MetricModel]:
        expected: dict[str, MetricModel] = {}
        for metric_name, metric in spec_model.metrics.items():
            if metric.deprecated:
                continue
            if combo.architecture in metric.support.unsupported_architectures:
                continue
            mode_support = normalize_support_value(metric.support.device_features.get(combo.device_mode))
            if mode_support is False:
                continue
            expected[f"{spec_model.metric_prefix}.{metric_name}"] = metric
        return expected

    def combo_filter(combo: ComboModel) -> str:
        parts = [f"gpu_architecture:{combo.architecture}"]
        if combo.device_mode == "mig":
            parts.append("gpu_slicing_mode:mig")
        elif combo.device_mode == "vgpu":
            parts.append("gpu_virtualization_mode:vgpu")
        else:
            parts.append("gpu_virtualization_mode:passthrough")
        return ",".join(parts)

    def combo_filter_expression(combo: ComboModel) -> str:
        parts = [f"gpu_architecture:{combo.architecture}"]
        if combo.device_mode == "mig":
            parts.append("gpu_slicing_mode:mig")
        elif combo.device_mode == "vgpu":
            parts.append("gpu_virtualization_mode:vgpu")
        else:
            parts.append("gpu_virtualization_mode:passthrough")
        return " AND ".join(parts)

    def _extract_tag_value(tag_set, key: str) -> str | None:
        for tag in tag_set or []:
            if isinstance(tag, str) and tag.startswith(f"{key}:"):
                return tag.split(":", 1)[1]
        return None

    def _extract_point_value(point) -> float | None:
        value = getattr(point, "_data_store", {}).get("value")
        if isinstance(value, (list, tuple)) and len(value) >= 2:
            return value[1]
        return None

    def query_device_count(api, combo: ComboModel, from_ts: int, to_ts: int) -> int:
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

    def _normalize_device_mode(slicing_mode: str | None, virtualization_mode: str | None) -> str:
        if (slicing_mode or "").lower() == "mig":
            return "mig"
        if (virtualization_mode or "").lower() == "vgpu":
            return "vgpu"
        return "physical"

    def discover_live_combos(api, from_ts: int, to_ts: int) -> set[tuple[str, str]]:
        query = "avg:gpu.device.total{*} by {gpu_architecture,gpu_slicing_mode,gpu_virtualization_mode}"
        response = api.query_metrics(_from=from_ts, to=to_ts, query=query)
        series_list = response.series if hasattr(response, "series") and response.series else []
        combos: set[tuple[str, str]] = set()
        for series in series_list:
            pointlist = getattr(series, "pointlist", []) or []
            if not any((value is not None and value > 0) for value in (_extract_point_value(p) for p in pointlist)):
                continue
            tag_set = getattr(series, "tag_set", [])
            arch = _extract_tag_value(tag_set, "gpu_architecture")
            slicing_mode = _extract_tag_value(tag_set, "gpu_slicing_mode")
            virtualization_mode = _extract_tag_value(tag_set, "gpu_virtualization_mode")
            if not arch:
                continue
            arch = arch.strip().lower()
            if arch in {"n/a", "na", "none", "unknown", ""}:
                continue
            combos.add((arch, _normalize_device_mode(slicing_mode, virtualization_mode)))
        return combos

    def get_expected_tags_for_metric(spec_model: SpecModel, metric: MetricModel) -> list[str]:
        tags: list[str] = []
        for tagset_name in metric.tagsets:
            tagset = spec_model.tagsets.get(tagset_name)
            if tagset:
                tags.extend(tagset.tags)
        tags.extend(metric.custom_tags)
        return list(dict.fromkeys(tags))

    def _chunk(items: list[str], chunk_size: int) -> list[list[str]]:
        return [items[i : i + chunk_size] for i in range(0, len(items), chunk_size)]

    def _normalize_group_value(value) -> str | None:
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

    def query_scalar_metrics_for_combo(
        api,
        metric_names: list[str],
        expected_tags_by_metric: dict[str, list[str]],
        combo_query_filter: str,
        from_ts: int,
        to_ts: int,
    ) -> tuple[set[str], dict[str, list[str]]]:
        if not metric_names:
            return set(), {}

        queries: list[Any] = []
        query_name_to_metric: dict[str, str] = {}
        for idx, metric_name in enumerate(metric_names):
            query_name = f"q{idx}"
            query = f"avg:{metric_name}{{{combo_query_filter}}}"
            expected_tags = expected_tags_by_metric.get(metric_name, [])
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
        attrs = getattr(getattr(response, "data", None), "attributes", None)
        columns = getattr(attrs, "columns", []) if attrs else []

        group_columns: dict[str, list] = {}
        number_columns: dict[str, list] = {}
        for column in columns or []:
            col_type = str(getattr(column, "type", "")).lower()
            col_name = getattr(column, "name", "")
            col_values = getattr(column, "values", []) or []
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

    def query_live_gpu_metrics_for_combo(api, combo: ComboModel, lookback: int, metric_prefix: str) -> set[str]:
        metrics: set[str] = set()
        filter_expr = combo_filter_expression(combo)
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
                metric_name = getattr(item, "id", "")
                if metric_name.startswith(f"{metric_prefix}."):
                    metrics.add(metric_name)
            pagination = getattr(getattr(response, "meta", None), "pagination", None)
            page_cursor = getattr(pagination, "next_cursor", None) if pagination else None
            if not page_cursor:
                break
        return metrics

    def validate_combo(
        metrics_api_v1,
        metrics_api_v2,
        spec_model: SpecModel,
        combo: ComboModel,
        from_ts: int,
        to_ts: int,
    ) -> ComboValidationResultModel:
        expected_metrics_map = get_expected_metrics_for_combo(spec_model, combo)
        expected_metrics = set(expected_metrics_map.keys())
        device_count = query_device_count(metrics_api_v1, combo, from_ts, to_ts)
        query_filter = combo_filter(combo)
        present_metrics: set[str] = set()
        unknown_metrics: set[str] = set()
        tag_failures: dict[str, list[str]] = {}
        if device_count > 0:
            metric_names = sorted(expected_metrics)
            expected_tags_by_metric: dict[str, list[str]] = {}
            for metric_name in metric_names:
                relative_name = metric_name.removeprefix(f"{spec_model.metric_prefix}.")
                metric_model = expected_metrics_map.get(metric_name) or spec_model.metrics[relative_name]
                expected_tags_by_metric[metric_name] = get_expected_tags_for_metric(spec_model, metric_model)
            for metric_batch in _chunk(metric_names, scalar_query_batch_size):
                batch_present, batch_failures = query_scalar_metrics_for_combo(
                    metrics_api_v2,
                    metric_batch,
                    expected_tags_by_metric,
                    query_filter,
                    from_ts,
                    to_ts,
                )
                present_metrics.update(batch_present)
                tag_failures.update(batch_failures)
            live_gpu_metrics = query_live_gpu_metrics_for_combo(
                metrics_api_v2,
                combo,
                max(to_ts - from_ts, 0),
                spec_model.metric_prefix,
            )
            unknown_metrics = live_gpu_metrics - expected_metrics
        return ComboValidationResultModel(
            combo=combo,
            device_count=device_count,
            expected_metrics=expected_metrics,
            present_metrics=present_metrics,
            unknown_metrics=unknown_metrics,
            tag_failures=tag_failures,
        )

    def combine_known_and_live_combos(
        known_combos: list[ComboModel],
        live_combo_keys: set[tuple[str, str]],
    ) -> list[ComboModel]:
        by_key: dict[tuple[str, str], ComboModel] = {(c.architecture, c.device_mode): c for c in known_combos}
        for key in sorted(live_combo_keys):
            if key not in by_key:
                by_key[key] = ComboModel(architecture=key[0], device_mode=key[1], is_known=False)
        return sorted(by_key.values(), key=lambda combo: (combo.architecture, combo.device_mode))

    spec_model = load_yaml_model(spec_path, SpecModel)
    architectures_model = load_yaml_model(architectures_path, ArchitecturesSpecModel)
    now = int(time.time())
    from_ts = now - int(lookback_seconds)
    known_combos = build_combinations(architectures_model)
    rows: list[dict[str, Any]] = []
    details: list[dict[str, Any]] = []
    failing_count = 0

    config = Configuration()
    config.server_variables["site"] = site
    with ApiClient(config) as api_client:
        metrics_api_v1 = MetricsApiV1(api_client)
        metrics_api_v2 = MetricsApiV2(api_client)
        live_combo_keys = discover_live_combos(metrics_api_v1, from_ts, now)
        combos = combine_known_and_live_combos(known_combos, live_combo_keys)
        for combo in combos:
            result = validate_combo(metrics_api_v1, metrics_api_v2, spec_model, combo, from_ts, now)
            if not combo.is_known and result.device_count == 0:
                continue

            if result.device_count == 0:
                status = "unknown" if not combo.is_known else "missing"
                known_metrics = len(result.expected_metrics) if combo.is_known else 0
                unknown_metrics = 0 if combo.is_known else len(result.expected_metrics)
                row = {
                    "architecture": combo.architecture,
                    "device_mode": combo.device_mode,
                    "status": status,
                    "found_devices": result.device_count,
                    "missing_metrics": 0,
                    "known_metrics": known_metrics,
                    "unknown_metrics": unknown_metrics,
                    "tag_failures": 0,
                }
            else:
                status = "unknown" if not combo.is_known else ("fail" if (result.missing_metrics or result.tag_failures) else "ok")
                row = {
                    "architecture": combo.architecture,
                    "device_mode": combo.device_mode,
                    "status": status,
                    "found_devices": result.device_count,
                    "missing_metrics": len(result.missing_metrics),
                    "known_metrics": len(result.expected_metrics),
                    "unknown_metrics": len(result.unknown_metrics),
                    "tag_failures": len(result.tag_failures),
                }
                details.append(
                    {
                        "label": f"{combo.architecture}/{combo.device_mode}",
                        "missing_metric_names": sorted(result.missing_metrics),
                        "unknown_metric_names": sorted(result.unknown_metrics),
                        "tag_failures": dict(sorted(result.tag_failures.items())),
                        "is_known": combo.is_known,
                    }
                )
                if combo.is_known and (result.missing_metrics or result.unknown_metrics or result.tag_failures):
                    failing_count += 1
            rows.append(row)

    return {
        "site": site,
        "metrics_count": len(spec_model.metrics),
        "architectures_count": len(architectures_model.architectures),
        "rows": rows,
        "details": details,
        "failing_count": failing_count,
    }


def _render_single_org(result: dict[str, Any], tabulate) -> None:
    print(f"Loaded metrics spec: {result['metrics_count']} entries")
    print(f"Loaded architecture spec: {result['architectures_count']} architectures")
    print(f"Querying Datadog API at {result['site']}...")

    print("\nSummary:")
    table_rows = [
        [
            row["architecture"],
            row["device_mode"],
            _color_status(row["status"]),
            row["found_devices"],
            _color_metric_counts(row["missing_metrics"], row["known_metrics"], row["unknown_metrics"]),
            _color_tag_failures(row["tag_failures"]),
        ]
        for row in result["rows"]
    ]
    print(
        tabulate(
            table_rows,
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

    for detail in result["details"]:
        if detail["missing_metric_names"] or detail["unknown_metric_names"] or detail["tag_failures"]:
            print(f"\n-- {detail['label']} --")
            if detail["missing_metric_names"]:
                print("  missing metric names:")
                for name in detail["missing_metric_names"]:
                    print(f"    - MISSING {name}")
            if detail["unknown_metric_names"]:
                print("  unknown metric names:")
                for name in detail["unknown_metric_names"]:
                    print(f"    - UNKNOWN {name}")
            if detail["tag_failures"]:
                print("  tag failure details:")
                for metric_name, tags in detail["tag_failures"].items():
                    print(f"    - TAG FAIL {metric_name}: missing/non-null [{', '.join(tags)}]")

    print(f"\nCombinations with metric/tag failures (and devices present): {result['failing_count']}")


def _render_merged(results_by_org: list[tuple[str, dict[str, Any]]], tabulate) -> None:
    precedence = {"fail": 4, "ok": 3, "unknown": 2, "missing": 1}
    by_combo: dict[tuple[str, str], dict[str, Any]] = {}
    total_failing = 0
    for _, result in results_by_org:
        total_failing += result["failing_count"]
        for row in result["rows"]:
            key = (row["architecture"], row["device_mode"])
            current = by_combo.get(key)
            if current is None:
                by_combo[key] = dict(row)
                continue
            if precedence.get(row["status"], 0) > precedence.get(current["status"], 0):
                current["status"] = row["status"]
            current["found_devices"] += row["found_devices"]
            current["missing_metrics"] = max(current["missing_metrics"], row["missing_metrics"])
            current["known_metrics"] = max(current["known_metrics"], row["known_metrics"])
            current["unknown_metrics"] = max(current["unknown_metrics"], row["unknown_metrics"])
            current["tag_failures"] = max(current["tag_failures"], row["tag_failures"])

    merged_rows: list[list[Any]] = []
    for architecture, device_mode in sorted(by_combo):
        row = by_combo[(architecture, device_mode)]
        merged_rows.append(
            [
                architecture,
                device_mode,
                _color_status(row["status"]),
                row["found_devices"],
                _color_metric_counts(row["missing_metrics"], row["known_metrics"], row["unknown_metrics"]),
                _color_tag_failures(row["tag_failures"]),
            ]
        )

    print("\nMerged summary:")
    print(
        tabulate(
            merged_rows,
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
    print("\nFailure counts per org:")
    for org, result in results_by_org:
        print(f"  - {org}: {result['failing_count']}")
    print(f"Total failures across orgs: {total_failing}")


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
    deps = _load_gpu_validation_deps()
    _require_api_keys()
    spec_path, architectures_path = _resolve_spec_paths(spec, architectures)
    result = _compute_validation(deps, spec_path, architectures_path, site, int(lookback_seconds))
    _render_single_org(result, deps["tabulate"])
    if result["failing_count"] > 0:
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
    deps = _load_gpu_validation_deps()
    spec_path, architectures_path = _resolve_spec_paths(spec, architectures)
    orgs = [("prod", "app.datadoghq.com"), ("staging", "ddstaging.datadoghq.com")]

    results_by_org: list[tuple[str, dict[str, Any]]] = []
    org_errors: list[str] = []
    for org_name, dd_auth_domain in orgs:
        print(f"\n== Running GPU validation for {org_name} ({dd_auth_domain}) ==")
        try:
            with dd_auth_api_app_keys(ctx, dd_auth_domain):
                _require_api_keys()
                result = _compute_validation(deps, spec_path, architectures_path, "datadoghq.com", int(lookback_seconds))
                results_by_org.append((org_name, result))
        except Exception as e:
            org_errors.append(f"{org_name}: {e}")
            print(f"[ERROR] {org_name} failed: {e}")

    if results_by_org:
        _render_merged(results_by_org, deps["tabulate"])

    if org_errors:
        print("\nOrg execution errors:")
        for err in org_errors:
            print(f"  - {err}")
        raise Exit(code=1)

    if any(result["failing_count"] > 0 for _, result in results_by_org):
        raise Exit(code=1)
