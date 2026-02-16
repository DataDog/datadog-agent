from __future__ import annotations
# Run with:
# dda inv --dep "datadog-api-client>=2.52.0" --dep "pydantic>=2.0" --dep "pyyaml>=6.0" --dep "tabulate>=0.9.0"

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.auth import dd_auth_api_app_keys
from tasks.libs.gpu.render import render_results
from tasks.libs.gpu.types import ValidationResults
from tasks.libs.gpu.validation import compute_validation, require_api_keys, resolve_spec_paths


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
    require_api_keys()
    spec_path, architectures_path = resolve_spec_paths(spec, architectures)
    result = compute_validation(spec_path, architectures_path, site, int(lookback_seconds), progress_writer=print)
    render_results(result)
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
    spec_path, architectures_path = resolve_spec_paths(spec, architectures)
    orgs = [("prod", "app.datadoghq.com"), ("staging", "ddstaging.datadoghq.com")]

    results: ValidationResults | None = None
    org_errors: list[str] = []
    for org_name, dd_auth_domain in orgs:
        print(f"\n== Running GPU validation for {org_name} ({dd_auth_domain}) ==")
        try:
            with dd_auth_api_app_keys(ctx, dd_auth_domain):
                require_api_keys()
                result = compute_validation(
                    spec_path,
                    architectures_path,
                    "datadoghq.com",
                    int(lookback_seconds),
                    progress_writer=print,
                )
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
