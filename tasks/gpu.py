from __future__ import annotations

import json
import shlex

from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.libs.common.auth import dd_auth_api_app_keys

VALIDATOR_PACKAGE = "./pkg/collector/corechecks/gpu/spec/metrics-validator"
VALIDATOR_BINARY = f"{VALIDATOR_PACKAGE}/gpu-metrics-validator"
VALIDATOR_SITE = "datadoghq.com"


def build_validator_binary(ctx) -> str:
    ctx.run(f"go build -o {shlex.quote(VALIDATOR_BINARY)} {VALIDATOR_PACKAGE}")
    return VALIDATOR_BINARY


def _select_orgs(org: str | None) -> list[tuple[str, str]]:
    orgs_by_name = {
        "prod": ("prod", "app.datadoghq.com"),
        "staging": ("staging", "ddstaging.datadoghq.com"),
    }
    if org is not None:
        return [orgs_by_name[org]]
    return list(orgs_by_name.values())


@task(
    name="validate-metrics",
    help={
        "lookback_seconds": "Metrics lookback window in seconds",
        "org": "Datadog org filter: prod, staging. If not provided, use all configured orgs",
    },
)
def validate_metrics(ctx, lookback_seconds=3600, org: str | None = None):
    """
    Validate live GPU metrics for the selected Datadog org(s).
    """
    from tasks.libs.gpu.render import render_results
    from tasks.libs.gpu.types import ValidationResults, validation_results_from_dict

    orgs = _select_orgs(org)

    print("== Building validator binary ==")
    binary_path = build_validator_binary(ctx)
    results: ValidationResults | None = None
    org_errors: list[str] = []
    for org_name, dd_auth_domain in orgs:
        print(f"\n== Running GPU validation for {org_name} ({dd_auth_domain}) ==")
        try:
            print(" - fetching API/App keys...")
            with dd_auth_api_app_keys(ctx, dd_auth_domain):
                with tempfile.NamedTemporaryFile(prefix="gpu-metrics-validator-", suffix=".json") as tmp:
                    command = (
                        f"{shlex.quote(binary_path)} "
                        "--mode metrics "
                        f"--site {shlex.quote(VALIDATOR_SITE)} "
                        f"--lookback-seconds {int(lookback_seconds)} "
                        f"--output-file {shlex.quote(tmp.name)}"
                    )
                    print(" - running validator...")
                    ctx.run(command)
                    result = validation_results_from_dict(json.load(tmp), site=VALIDATOR_SITE)

                if results is None:
                    results = result
                else:
                    results.update(result)
        except Exception as e:
            org_errors.append(f"{org_name}: {e}")
            print(f"[ERROR] {org_name} failed: {e}")

    if results:
        render_results(results)

    if org_errors:
        print("\nOrg execution errors:")
        for err in org_errors:
            print(f"  - {err}")
        raise Exit(code=1)

    if results and results.failing_count > 0:
        raise Exit(code=1)


@task(
    name="validate-tags",
    help={
        "window_seconds": "All-tags lookup window in seconds (defaults to 14400 / 4 hours)",
        "org": "Datadog org filter: prod, staging. If not provided, use all configured orgs",
        "metric_name_filter": "Only validate metrics whose full name contains this substring",
        "tag_name_filter": "Only validate spec tag names containing this substring",
        "filter_tags": "Optional all-tags endpoint filter[tags] expression",
    },
)
def validate_tags(
    ctx,
    window_seconds=14400,
    org: str | None = None,
    metric_name_filter=None,
    tag_name_filter=None,
    filter_tags=None,
):
    """
    Validate GPU metric tag values against regexes from tags.yaml for the selected Datadog org(s).
    """
    from tasks.libs.gpu.render import render_tag_validation_results

    orgs = _select_orgs(org)
    binary_path = build_validator_binary(ctx)

    all_failures: dict[str, dict[str, list[str]]] = {}
    org_errors: list[str] = []
    for org_name, dd_auth_domain in orgs:
        print(f"\n== Running GPU tag validation for {org_name} ({dd_auth_domain}) ==")
        try:
            with dd_auth_api_app_keys(ctx, dd_auth_domain):
                command = [
                    shlex.quote(binary_path),
                    "--mode",
                    "tags",
                    "--site",
                    shlex.quote(VALIDATOR_SITE),
                    "--window-seconds",
                    str(int(window_seconds)),
                ]
                if metric_name_filter:
                    command.extend(["--metric-name-filter", shlex.quote(metric_name_filter)])
                if tag_name_filter:
                    command.extend(["--tag-name-filter", shlex.quote(tag_name_filter)])
                if filter_tags:
                    command.extend(["--filter-tags", shlex.quote(filter_tags)])

                payload = json.loads(ctx.run(" ".join(command), hide=True).stdout)
                failures = payload.get("failures", {})
                errors = payload.get("errors", [])
                for metric_name, tags_for_metric in failures.items():
                    target = all_failures.setdefault(metric_name, {})
                    for tag_name, values in tags_for_metric.items():
                        target[tag_name] = sorted(set(target.get(tag_name, [])) | set(values))
                org_errors.extend(f"{org_name}: {err}" for err in errors)
        except Exception as e:
            org_errors.append(f"{org_name}: {e}")
            print(f"[ERROR] {org_name} failed: {e}")

    render_tag_validation_results(VALIDATOR_SITE, all_failures, org_errors)
    if all_failures or org_errors:
        raise Exit(code=1)
