from __future__ import annotations

import json
import os
import re
import shlex
import tempfile
import traceback
from pathlib import Path

import requests
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.libs.common.auth import datadog_infra_token, dd_auth_api_app_keys
from tasks.schema.generate import schema_codegen

SPEC_PACKAGE = "./pkg/collector/corechecks/gpu/spec"
ALLOWLIST_PACKAGE = f"{SPEC_PACKAGE}/allowlist"
ALLOWLIST_BINARY = f"{ALLOWLIST_PACKAGE}/gpu-metrics-allowlist"
DEFAULT_ALLOWLIST_PATH = "../dd-analytics/luigiscripts/billing/usage/standard_metric_allowlist.json"
METADATA_PACKAGE = f"{SPEC_PACKAGE}/metadata"
METADATA_BINARY = f"{METADATA_PACKAGE}/gpu-metrics-metadata"
DEFAULT_METADATA_PATH = "../integrations-core/gpu/metadata.csv"
METRICS_LIST_PACKAGE = f"{SPEC_PACKAGE}/metrics-list"
METRICS_LIST_BINARY = f"{METRICS_LIST_PACKAGE}/gpu-metrics-list"
DEFAULT_METRICS_LIST_PATH = "gpu_metrics.tsv"
VALIDATOR_PACKAGE = f"{SPEC_PACKAGE}/metrics-validator"
VALIDATOR_BINARY = f"{VALIDATOR_PACKAGE}/gpu-metrics-validator"
VALIDATOR_SITE = "datadoghq.com"
RELEASE_BRANCH_RE = re.compile(r"^(?P<major>\d+)\.(?P<minor>\d+)\.x$")
RELEASE_CANDIDATE_TAG_RE = re.compile(r"^(?P<version>\d+\.\d+\.(?P<patch>\d+))-rc\.?\d+$")
GPU_BURNER_BRANCH = "main"
GPU_BURNER_VERSION = "87719309"
MASS_READ_URL = "https://mass-read.us1.ddbuild.io/internal/artifact"
MASS_AUDIENCE = "rapid-dependency-management-mass"


def get_gpu_burner_artifact_path() -> str:
    branch = os.environ.get("GPU_BURNER_BRANCH", GPU_BURNER_BRANCH)
    version = os.environ.get("GPU_BURNER_VERSION", GPU_BURNER_VERSION)
    return f"gpu-burner/branches/{branch}/{version}/gpu-burner.tar.gz"


def build_binary(ctx, package: str, output_path: str, label: str) -> str:
    print(f"== Building {label} binary ==")

    # TODO: remove once Bazel is used to build the Agent
    schema_codegen(ctx)

    ctx.run(f"go build -o {shlex.quote(output_path)} {package}")
    return output_path


def agent_version_from_branch(ctx) -> str:
    branch = ctx.run("git branch --show-current", hide=True).stdout.strip()
    branch_match = RELEASE_BRANCH_RE.fullmatch(branch)
    if branch_match is None:
        raise Exit(message=f"cannot derive agent version: branch {branch!r} is not a release branch")

    tag_pattern = f"{branch_match['major']}.{branch_match['minor']}.*"
    tags = ctx.run(f"git tag --list {shlex.quote(tag_pattern)} --sort=-v:refname", hide=True).stdout.splitlines()
    if not tags:
        ctx.run("git fetch --tags")
        tags = ctx.run(f"git tag --list {shlex.quote(tag_pattern)} --sort=-v:refname", hide=True).stdout.splitlines()
    if not tags:
        raise Exit(message=f"cannot derive agent version: no tags found for branch {branch!r}")

    tag = tags[0]
    tag_match = RELEASE_CANDIDATE_TAG_RE.fullmatch(tag)
    if tag_match is None:
        raise Exit(message=f"cannot derive agent version: latest tag {tag!r} is not a release candidate")

    version = tag_match["version"]
    if tag_match["patch"] == "0":
        return version.rsplit(".", 1)[0] + ".*"
    return version + "*"


@task(
    name="download-gpu-burner",
    help={
        "output_path": "Directory where the gpu-burner archive is extracted",
    },
)
def download_gpu_burner(ctx, output_path: str):
    """
    Download and extract gpu-burner from mASS.

    Uses authanywhere in CI and ddtool when run locally.
    """
    destination = Path(output_path).resolve()
    destination.mkdir(parents=True, exist_ok=True)
    gpu_burner_artifact_path = get_gpu_burner_artifact_path()
    artifact_url = f"{MASS_READ_URL}/{gpu_burner_artifact_path}"
    print("== Fetching mASS authorization token ==", flush=True)
    token = datadog_infra_token(ctx, audience=MASS_AUDIENCE)

    print(f"== Downloading gpu-burner to {destination} ==", flush=True)
    with tempfile.NamedTemporaryFile(prefix="gpu-burner-", suffix=".tar.gz") as archive:
        try:
            response = requests.get(
                artifact_url,
                headers={"Authorization": token},
                stream=True,
                timeout=None,
            )
            response.raise_for_status()
        except requests.RequestException as e:
            raise Exit(message=f"failed to download gpu-burner from mASS: {e}") from e

        with open(archive.name, "wb") as writer:
            for chunk in response.iter_content(chunk_size=8192):
                if chunk:
                    writer.write(chunk)

        print(f"== Extracting gpu-burner to {destination} ==", flush=True)
        ctx.run(f"tar -xzf {shlex.quote(archive.name)} -C {shlex.quote(str(destination))}")


@task(
    name="validate-metrics",
    help={
        "lookback_seconds": "Metrics lookback window in seconds",
        "org": "Datadog org filter: prod, staging. If not provided, use all configured orgs",
        "agent_version": "Agent image-tag wildcard to validate (default: derive from the current release branch)",
        "metric_filter": "Additional Datadog metric filter expression, ANDed with the GPU config filter",
    },
)
def validate_metrics(
    ctx,
    lookback_seconds=3600,
    org: str | None = None,
    agent_version: str | None = None,
    metric_filter: str | None = None,
):
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

    if agent_version is None:
        agent_version = agent_version_from_branch(ctx)

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
                if agent_version:
                    command += f" --agent-version {shlex.quote(agent_version)}"
                if metric_filter:
                    command += f" --metric-filter {shlex.quote(metric_filter)}"
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


@task(
    name="update-metadata",
    help={
        "metadata_path": f"Path to gpu/metadata.csv (default: {DEFAULT_METADATA_PATH})",
        "default_interval": "Default interval value for metrics missing metadata.interval",
        "include_histograms": "Include histogram metrics in metadata.csv",
    },
)
def update_metadata(
    ctx,
    metadata_path: str = DEFAULT_METADATA_PATH,
    default_interval: int = 16,
    include_histograms: bool = False,
):
    """
    Update integrations-core GPU metadata.csv entries from the shared GPU spec.
    """
    binary_path = build_binary(ctx, METADATA_PACKAGE, METADATA_BINARY, "metadata updater")
    command = (
        f"{shlex.quote(binary_path)} "
        f"--metadata-path {shlex.quote(metadata_path)} "
        f"--default-interval {int(default_interval)}"
    )
    if include_histograms:
        command += " --include-histograms"
    print(f"== Updating GPU metadata at {metadata_path} ==")
    ctx.run(command)


@task(
    name="generate-metrics-list",
    help={
        "output_path": f"Path to write generated TSV (default: {DEFAULT_METRICS_LIST_PATH})",
    },
)
def generate_metrics_list(
    ctx,
    output_path: str = DEFAULT_METRICS_LIST_PATH,
):
    """
    Generate a GPU metrics list TSV from the shared GPU spec.
    """
    binary_path = build_binary(ctx, METRICS_LIST_PACKAGE, METRICS_LIST_BINARY, "metrics list generator")
    command = f"{shlex.quote(binary_path)} " f"--output-path {shlex.quote(output_path)}"
    print(f"== Generating GPU metrics list TSV at {output_path} ==")
    ctx.run(command)
