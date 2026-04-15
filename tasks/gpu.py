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
def validate_metrics(
    ctx,
    lookback_seconds=3600,
    org: str | None = None,
):
    """
    Validate live GPU metrics for the selected Datadog org(s).
    """
    from tasks.libs.gpu.render import render_results
    from tasks.libs.gpu.types import ValidationResults, validation_results_from_dict

    orgs = _select_orgs(org)
    lookback_seconds = int(lookback_seconds)

    print("== Building validator binary ==")
    binary_path = build_validator_binary(ctx)
    results: ValidationResults | None = None
    org_errors: list[str] = []
    for org_name, dd_auth_domain in orgs:
        print(f"\n== Running GPU validation for {org_name} ({dd_auth_domain}) ==")
        try:
            print(" - fetching API/App keys...")
            with dd_auth_api_app_keys(ctx, dd_auth_domain):
                print(" - running validator...")
                command = [
                    shlex.quote(binary_path),
                    "--site",
                    shlex.quote(VALIDATOR_SITE),
                    "--lookback-seconds",
                    str(lookback_seconds),
                ]
                payload = json.loads(ctx.run(" ".join(command), hide=True).stdout)
                result = validation_results_from_dict(payload, site=VALIDATOR_SITE)

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
