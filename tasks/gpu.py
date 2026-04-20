from __future__ import annotations

import json
import shlex
import tempfile
import traceback

from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.libs.common.auth import dd_auth_api_app_keys

SPEC_PACKAGE = "./pkg/collector/corechecks/gpu/spec"
ALLOWLIST_PACKAGE = f"{SPEC_PACKAGE}/allowlist"
ALLOWLIST_BINARY = f"{ALLOWLIST_PACKAGE}/gpu-metrics-allowlist"
DEFAULT_ALLOWLIST_PATH = "../dd-analytics/luigiscripts/billing/usage/standard_metric_allowlist.json"
VALIDATOR_PACKAGE = f"{SPEC_PACKAGE}/metrics-validator"
VALIDATOR_BINARY = f"{VALIDATOR_PACKAGE}/gpu-metrics-validator"
VALIDATOR_SITE = "datadoghq.com"


def build_binary(ctx, package: str, output_path: str, label: str) -> str:
    print(f"== Building {label} binary ==")
    ctx.run(f"go build -o {shlex.quote(output_path)} {package}")
    return output_path


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

    orgs_by_name = {
        "prod": ("prod", "app.datadoghq.com"),
        "staging": ("staging", "ddstaging.datadoghq.com"),
    }

    if org is not None:
        orgs = [orgs_by_name[org]]
    else:
        orgs = list(orgs_by_name.values())

    binary_path = build_binary(ctx, VALIDATOR_PACKAGE, VALIDATOR_BINARY, "validator")
    results: ValidationResults | None = None
    org_errors: list[str] = []
    for org_name, dd_auth_domain in orgs:
        print(f"\n== Running GPU validation for {org_name} ({dd_auth_domain}) ==")
        try:
            print(" - fetching API/App keys...")
            with (
                dd_auth_api_app_keys(ctx, dd_auth_domain) as _,
                tempfile.NamedTemporaryFile(prefix="gpu-metrics-validator-", suffix=".json") as tmp,
            ):
                command = (
                    f"{shlex.quote(binary_path)} "
                    f"--site {shlex.quote(VALIDATOR_SITE)} "
                    f"--lookback-seconds {int(lookback_seconds)} "
                    f"--output-file {shlex.quote(tmp.name)}"
                )
                print(" - running validator...")
                res = ctx.run(command, warn=True)
                result = validation_results_from_dict(json.load(tmp), site=VALIDATOR_SITE)

                if results is None:
                    results = result
                else:
                    results.update(result)

                if not res.ok:
                    raise RuntimeError(f"validator failed: {res.stderr}")
        except Exception as e:
            org_errors.append(f"{org_name}: {e}\nStack trace:\n{traceback.format_exc()}")

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
    name="update-metrics-allowlist",
    help={
        "allowlist_path": f"Path to standard_metric_allowlist.json (default: {DEFAULT_ALLOWLIST_PATH})",
    },
)
def update_metrics_allowlist(ctx, allowlist_path: str = DEFAULT_ALLOWLIST_PATH):
    """
    Update the GPU metrics entries in the standard metric allowlist.
    """
    binary_path = build_binary(ctx, ALLOWLIST_PACKAGE, ALLOWLIST_BINARY, "allowlist updater")
    command = f"{shlex.quote(binary_path)} " f"--allowlist-path {shlex.quote(allowlist_path)}"
    print(f"== Updating GPU metric allowlist at {allowlist_path} ==")
    ctx.run(command)
