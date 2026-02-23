from __future__ import annotations

import os
import time
from pathlib import Path
from typing import Any

from invoke import task
from invoke.exceptions import Exit


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
            "  dda inv --dep datadog-api-client>=2.20.0 --dep pydantic>=2.0 --dep pyyaml>=6.0 --dep tabulate>=0.9.0 gpu.validate-live",
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


@task(
    help={
        "spec": "Path to gpu_metrics.yaml",
        "architectures": "Path to architectures.yaml",
        "site": "Datadog site (defaults to DD_SITE or datadoghq.com)",
        "lookback_seconds": "Metrics lookback window in seconds",
    }
)
def validate_live(_, spec=None, architectures=None, site=None, lookback_seconds=3600):
    """
    Validate live GPU metrics by architecture/device-mode combinations.
    """
    deps = _load_gpu_validation_deps()
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
    tabulate = deps["tabulate"]
    yaml = deps["yaml"]

    DEVICE_MODES = ("physical", "mig", "vgpu")
    # Staging rejects scalar payloads above ~50 queries with HTTP 422.
    SCALAR_QUERY_BATCH_SIZE = 50
    ANSI_GREEN = "\033[32m"
    ANSI_RED = "\033[31m"
    ANSI_YELLOW = "\033[33m"
    ANSI_RESET = "\033[0m"

    def color_status(status: str) -> str:
        if status == "ok":
            return f"{ANSI_GREEN}{status}{ANSI_RESET}"
        if status == "fail":
            return f"{ANSI_RED}{status}{ANSI_RESET}"
        return f"{ANSI_YELLOW}{status}{ANSI_RESET}"

    def color_metric_counts(missing: int, known: int, unknown: int) -> str:
        missing_str = f"{ANSI_RED}{missing}{ANSI_RESET}" if missing > 0 else str(missing)
        known_str = f"{ANSI_GREEN}{known}{ANSI_RESET}" if known > 0 else str(known)
        unknown_str = f"{ANSI_YELLOW}{unknown}{ANSI_RESET}" if unknown > 0 else str(unknown)
        return f"{missing_str}/{known_str}/{unknown_str}"

    def color_tag_failures(count: int) -> str:
        if count > 0:
            return f"{ANSI_RED}{count}{ANSI_RESET}"
        return str(count)

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

    def make_api_client(site_name: str):
        config = Configuration()
        config.server_variables["site"] = site_name
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
            # Ignore discovered groups that did not report a positive device count.
            if not any((value is not None and value > 0) for value in (_extract_point_value(p) for p in pointlist)):
                continue
            tag_set = getattr(series, "tag_set", [])
            arch = _extract_tag_value(tag_set, "gpu_architecture")
            slicing_mode = _extract_tag_value(tag_set, "gpu_slicing_mode")
            virtualization_mode = _extract_tag_value(tag_set, "gpu_virtualization_mode")
            if not arch:
                continue
            arch = arch.strip().lower()
            # Drop placeholder architecture values at discovery time.
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

    def query_live_gpu_metrics_for_combo(
        api,
        combo: ComboModel,
        lookback_seconds: int,
        metric_prefix: str,
    ) -> set[str]:
        metrics: set[str] = set()
        filter_expr = combo_filter_expression(combo)
        page_cursor = None
        while True:
            kwargs = {
                "filter_tags": filter_expr,
                "filter_queried": True,
                "window_seconds": max(lookback_seconds, 3600),
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

            for batch_index, metric_batch in enumerate(_chunk(metric_names, SCALAR_QUERY_BATCH_SIZE), start=1):
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
                print(
                    f"    Scalar query batches: {batch_index}/{(len(metric_names) + SCALAR_QUERY_BATCH_SIZE - 1) // SCALAR_QUERY_BATCH_SIZE}..."
                )
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

    repo_root = Path(__file__).resolve().parents[1]
    spec_path = spec or str(repo_root / "pkg" / "collector" / "corechecks" / "gpu" / "spec" / "gpu_metrics.yaml")
    architectures_path = architectures or str(
        repo_root / "pkg" / "collector" / "corechecks" / "gpu" / "spec" / "architectures.yaml"
    )

    if not Path(spec_path).exists():
        raise Exit(f"Spec file not found: {spec_path}", code=1)
    if not Path(architectures_path).exists():
        raise Exit(f"Architectures file not found: {architectures_path}", code=1)

    spec_model = load_spec(spec_path)
    architectures_model = load_architectures(architectures_path)
    site_name = site or os.environ.get("DD_SITE", "datadoghq.com")

    print(f"Loaded metrics spec: {len(spec_model.metrics)} entries")
    print(f"Loaded architecture spec: {len(architectures_model.architectures)} architectures")
    print(f"Querying Datadog API at {site_name}...")

    now = int(time.time())
    from_ts = now - int(lookback_seconds)
    failing_count = 0
    summary_rows: list[list[str | int]] = []
    known_combos = build_combinations(architectures_model)

    config = make_api_client(site_name)
    with ApiClient(config) as api_client:
        metrics_api_v1 = MetricsApiV1(api_client)
        metrics_api_v2 = MetricsApiV2(api_client)
        live_combo_keys = discover_live_combos(metrics_api_v1, from_ts, now)
        combos = combine_known_and_live_combos(known_combos, live_combo_keys)
        print(f"Generated combinations: {len(combos)} ({len(known_combos)} known, {len(combos) - len(known_combos)} discovered)")
        for combo in combos:
            label = f"{combo.architecture}/{combo.device_mode}"
            print(f"\n-- {label} --")
            result = validate_combo(metrics_api_v1, metrics_api_v2, spec_model, combo, from_ts, now)
            if not combo.is_known and result.device_count == 0:
                print("  discovered combo ignored (no devices)")
                continue
            print(f"  devices: {result.device_count}")
            print(f"  expected metrics: {len(result.expected_metrics)}")
            if result.device_count == 0:
                print("  present metrics: skipped (no devices)")
                print("  missing metrics: skipped (no devices)")
                known_metrics = len(result.expected_metrics) if combo.is_known else 0
                unknown_metrics = 0 if combo.is_known else len(result.expected_metrics)
                summary_rows.append(
                    [
                        combo.architecture,
                        combo.device_mode,
                        color_status("unknown" if not combo.is_known else "missing"),
                        result.device_count,
                        color_metric_counts(0, known_metrics, unknown_metrics),
                        color_tag_failures(0),
                    ]
                )
                continue
            print(f"  present metrics: {len(result.present_metrics)}")
            print(f"  missing metrics: {len(result.missing_metrics)}")
            print(f"  unknown metrics: {len(result.unknown_metrics)}")
            print(f"  tag failures: {len(result.tag_failures)}")
            summary_rows.append(
                [
                    combo.architecture,
                    combo.device_mode,
                    color_status(
                        "unknown"
                        if not combo.is_known
                        else ("fail" if (result.missing_metrics or result.tag_failures) else "ok")
                    ),
                    result.device_count,
                    color_metric_counts(
                        len(result.missing_metrics),
                        len(result.expected_metrics),
                        len(result.unknown_metrics),
                    ),
                    color_tag_failures(len(result.tag_failures)),
                ]
            )
            if result.missing_metrics:
                print("  missing metric names:")
                for name in sorted(result.missing_metrics):
                    print(f"    - MISSING {name}")
            if result.unknown_metrics:
                print("  unknown metric names:")
                for name in sorted(result.unknown_metrics):
                    print(f"    - UNKNOWN {name}")
            if result.tag_failures:
                print("  tag failure details:")
                for metric_name in sorted(result.tag_failures):
                    tags = ", ".join(result.tag_failures[metric_name])
                    print(f"    - TAG FAIL {metric_name}: missing/non-null [{tags}]")
            if combo.is_known and (result.missing_metrics or result.unknown_metrics or result.tag_failures):
                failing_count += 1

    print("\nSummary:")
    print(
        tabulate(
            summary_rows,
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

    print(f"\nCombinations with metric/tag failures (and devices present): {failing_count}")
    if failing_count > 0:
        raise Exit(code=1)
