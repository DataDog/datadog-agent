import os
import tempfile
from datetime import datetime

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.diff import diff as _diff
from tasks.libs.common.download import download as _download
from tasks.libs.common.git import get_default_branch
from tasks.libs.package.extract import extract_deb, extract_rpm
from tasks.libs.package.size import (
    PACKAGE_SIZE_TEMPLATE,
    compare,
    compute_package_size_metrics,
)
from tasks.libs.package.url import get_deb_package_url, get_rpm_package_url
from tasks.libs.package.utils import (
    PackageSize,
    get_ancestor,
    get_package_name,
    list_packages,
    retrieve_package_sizes,
    upload_package_sizes,
)
from tasks.libs.pipeline.utils import get_pipeline_id


def parse_job_name(job_name: str) -> tuple[str, str, str, str]:
    """
    Parse a pipeline name to extract binary, type, flavor, and arch.

    Supports pipeline name formats like:
    - agent_deb_amd64
    - agent_rpm_amd64_fips
    - dd-gitlab/agent_deb-arm64-a7-fips
    - dd-gitlab/cluster_agent_fips-build_amd64

    Returns: (binary, type, flavor, arch)
    Raises: Exit if pipeline name cannot be parsed
    """
    # Remove dd-gitlab prefix if present
    name = job_name.replace("dd-gitlab/", "")

    # Replace hyphens with underscores for consistent parsing
    name = name.replace("-", "_")

    # Remove common suffixes that don't affect parsing
    for suffix in ["a7", "build", "test", "static", "binary", "build_test", "standalone"]:
        name = name.replace(f"_{suffix}", "")

    parts = name.split("_")

    # Known values
    binaries = {"agent", "dogstatsd", "iot_agent", "ddot", "cluster_agent", "cws_instrumentation", "host_profiler"}
    types = {"deb", "rpm", "msi", "docker", "dmg", "oci", "suse"}
    flavors = {"fips", "heroku", "jmx", "cloudfoundry", "ota"}
    archs = {"amd64", "x64", "x86", "x86_64", "arm64", "armhf"}

    binary = ""
    _type = ""
    flavor = ""
    arch = ""

    # First pass: identify binary, type, flavor, arch from known values
    i = 0
    while i < len(parts):
        part = parts[i]

        # Check for binary (handle multi-word binaries like iot_agent, cluster_agent)
        if not binary:
            if part == "iot" and i + 1 < len(parts) and parts[i + 1] == "agent":
                binary = "iot_agent"
                i += 2
                continue
            elif part == "cluster" and i + 1 < len(parts) and parts[i + 1] == "agent":
                binary = "cluster_agent"
                i += 2
                continue
            elif part == "cws" and i + 1 < len(parts) and parts[i + 1] == "instrumentation":
                binary = "cws_instrumentation"
                i += 2
                continue
            elif part == "host" and i + 1 < len(parts) and parts[i + 1] == "profiler":
                binary = "host_profiler"
                i += 2
                continue
            elif part in binaries:
                binary = part
                i += 1
                continue

        # Check for type
        if not _type and part in types:
            _type = part
            i += 1
            continue

        # Check for flavor
        if part in flavors:
            flavor = part
            i += 1
            continue

        # Check for architecture (handle x64 -> amd64 conversion)
        if part in archs:
            if part == "x64":
                arch = "amd64"
            else:
                arch = part
            i += 1
            continue

        i += 1

    # Special cases for docker pipelines
    if "docker" in parts or "docker_build" in job_name:
        if not _type:
            _type = "docker"
        if not binary:
            # Try to find binary in the original name
            if "agent" in job_name and "cluster" not in job_name and "dogstatsd" not in job_name:
                binary = "agent"
            elif "cluster_agent" in job_name:
                binary = "cluster_agent"
            elif "dogstatsd" in job_name:
                binary = "dogstatsd"

    # Validate and set defaults
    if not binary:
        raise Exit(code=1, message=f"Could not determine binary from pipeline name: {job_name}")

    if not _type:
        # Try to infer type from binary or pipeline name
        if "deb" in job_name:
            _type = "deb"
        elif "rpm" in job_name:
            _type = "rpm"
        elif "docker" in job_name:
            _type = "docker"
        elif "msi" in job_name:
            _type = "msi"
        elif "dmg" in job_name:
            _type = "dmg"
        elif "oci" in job_name:
            _type = "oci"
        else:
            raise Exit(code=1, message=f"Could not determine package type from pipeline name: {job_name}")

    if not arch:
        raise Exit(code=1, message=f"Could not determine architecture from pipeline name: {job_name}")

    return binary, _type, flavor, arch


@task
def check_size(ctx, filename: str = 'package_sizes.json', dry_run: bool = False):
    package_sizes = retrieve_package_sizes(ctx, filename, distant=not dry_run)
    on_main = os.environ['CI_COMMIT_REF_NAME'] == get_default_branch()
    ancestor = get_ancestor(ctx, package_sizes, on_main)
    if on_main:
        # Initialize to default values
        if ancestor in package_sizes:
            # The test already ran on this commit
            return
        package_sizes[ancestor] = PACKAGE_SIZE_TEMPLATE.copy()
        package_sizes[ancestor]['timestamp'] = int(datetime.now().timestamp())
    # Check size of packages
    print(
        color_message(f"Checking package sizes from {os.environ['CI_COMMIT_REF_NAME']} against {ancestor}", Color.BLUE)
    )
    size_table = []
    for package_info in list_packages(PACKAGE_SIZE_TEMPLATE):
        pkg_size = PackageSize(*package_info)
        size_table.append(compare(ctx, package_sizes, ancestor, pkg_size))

    if on_main:
        upload_package_sizes(ctx, package_sizes, filename, distant=not dry_run)


@task
def send_size(
    ctx,
    flavor: str,
    package_os: str,
    package_path: str,
    git_ref: str,
    bucket_branch: str,
    arch: str,
    send_series: bool = True,
):
    """
    For a provided package path, os and flavor, retrieves size information on the package and its included
    Agent binaries, prints them, and sends them to Datadog.

    The --git-ref, --bucket-branch, and --arch parameters are used to add tags to the metrics.

    Needs the DD_API_KEY environment variable to be set. Needs native utilities for the given os
    to be present (du in all cases, dpkg for debian, rpm2cpio and cpio for centos/suse).

    Use --no-send-series to skip the metrics submission part (and the need for a DD_API_KEY).
    """

    from tasks.libs.common.datadog_api import send_metrics

    if not os.path.exists(package_path):
        raise Exit(code=1, message=color_message(f"Package not found at path {package_path}", "orange"))

    if send_series and not os.environ.get("DD_API_KEY"):
        raise Exit(
            code=1,
            message=color_message(
                "DD_API_KEY environment variable not set, cannot send pipeline metrics to the backend", "red"
            ),
        )

    series = compute_package_size_metrics(
        ctx=ctx,
        flavor=flavor,
        package_os=package_os,
        package_path=package_path,
        git_ref=git_ref,
        bucket_branch=bucket_branch,
        arch=arch,
    )

    print(color_message("Data collected:", "blue"))
    print(series)
    if send_series:
        print(color_message("Sending metrics to Datadog", "blue"))
        send_metrics(series=series)
        print(color_message("Done", "green"))


@task(
    help={
        "binary": "The binary of the package, either agent or dogstatsd",
        "type": "The type of package, only deb is supported for now",
        "flavor": "The flavor of the package, either empty, heroku, iot or fips",
        "arch": "The package architecture, either amd64 or arm64",
        "pull_request_id": "Compare the package from the pull request's head to its merge-base.",
        "base_ref": "The base ref to compare the package from.",
        "base_pipeline": "The base pipeline id to compare the package from.",
        "target_ref": "The target ref to compare the package to.",
        "target_pipeline": "The target pipeline id to compare the package to.",
    }
)
def diff(
    ctx: Context,
    binary: str = "agent",
    _type: str = "deb",
    flavor: str = "",
    arch: str = "amd64",
    pull_request_id: str | None = None,
    base_ref: str | None = None,
    base_pipeline: str | None = None,
    target_ref: str | None = None,
    target_pipeline: str | None = None,
    sort_by_size: bool = True,
):
    """
    Diff the content of the given package.

    Exactly one of --base-ref, --base-pipeline, or --pull-request-id must be provided.
    Exactly one of --target-ref, --target-pipeline, or --pull-request-id must be provided.
    """

    repo = get_gitlab_repo("DataDog/datadog-agent")
    base_pipeline_id = get_pipeline_id(ctx, repo, base_ref, base_pipeline, pull_request_id, base=True)
    target_pipeline_id = get_pipeline_id(ctx, repo, target_ref, target_pipeline, pull_request_id, base=False)
    print(f"Comparing package from pipeline {base_pipeline_id} with pipeline {target_pipeline_id}")

    tmpdir = tempfile.mkdtemp()
    print(f"Artifacts will be downloaded to {tmpdir}")

    base_extract_dir = os.path.join(tmpdir, "base")
    download(ctx, base_pipeline_id, binary, _type, flavor, arch, tmpdir, extract_dir=base_extract_dir)
    target_extract_dir = os.path.join(tmpdir, "target")
    download(ctx, target_pipeline_id, binary, _type, flavor, arch, tmpdir, extract_dir=target_extract_dir)

    _diff(base_extract_dir, target_extract_dir, sort_by_size=sort_by_size)


@task(
    help={
        "job_name": "The pipeline job name to download from (e.g., agent_deb_amd64, docker_agent_jmx_arm64)",
        "pull_request_id": "Pull request ID (optional, for downloading from PR pipeline)",
        "ref": "Git ref to download from (optional, alternative to pull_request_id)",
        "pipeline_id": "Pipeline ID (optional, alternative to ref or pull_request_id)",
        "output": "(optional) The path to download the package to, otherwise use the file name",
    }
)
def download_by_test(
    ctx: Context,
    job_name: str,
    pull_request_id: str | None = None,
    ref: str | None = None,
    pipeline_id: str | None = None,
    output: str | None = None,
):
    """
    Download a package from a CI pipeline, specified by pipeline name.

    The pipeline name is parsed to extract binary, type, flavor, and architecture.
    You must provide exactly one of: pull_request_id, ref, or pipeline_id.

    Examples:
        dda inv package.download-by-test --job-name=agent_deb_amd64 --pull-request-id=12345
        dda inv package.download-by-test --job-name=agent_rpm_arm64_fips --ref=main
    """

    # Parse the pipeline name
    binary, _type, flavor, arch = parse_job_name(job_name=job_name)

    # Determine the pipeline ID
    if pipeline_id is None:
        repo = get_gitlab_repo("DataDog/datadog-agent")
        pipeline_id = get_pipeline_id(ctx, repo, ref, None, pull_request_id, base=False)
    else:
        pipeline_id = int(pipeline_id)

    print(f"Downloading {binary} ({_type}) {flavor or 'default'} {arch} from pipeline {pipeline_id}")
    download(ctx, pipeline_id, binary, _type, flavor, arch, path=output, extract=False, extract_dir=None)


@task(
    help={
        "pipeline": "The pipeline id to download the package from",
        "binary": "The binary of the package, either agent or dogstatsd",
        "type": "The type of package",
        "flavor": "The flavor of the package, either empty, heroku, iot or fips",
        "arch": "The package architecture, either amd64 or arm64",
        "path": "The path to download the package to",
        "extract": "Whether to extract the package. This is only supported if type=deb.",
        "extract_dir": "The directory to extract the package to",
    }
)
def download(
    ctx: Context,
    pipeline: str | int,
    binary: str = "agent",
    _type: str = "deb",
    flavor: str = "",
    arch: str = "amd64",
    path: str | None = None,
    extract: bool = True,
    extract_dir: str | None = None,
):
    """
    Download the package from the given pipeline.
    """

    assert binary in ("agent", "dogstatsd"), "Unknown binary"
    assert flavor in ("", "heroku", "iot", "fips"), "Unknown flavor"
    assert arch in ("amd64", "arm64"), "Unknown architecture"

    if path is None:
        path = os.getcwd()

    if extract_dir is None:
        extract_dir = path
    else:
        # If extract_dir is provided, always extract
        extract = True

    match _type:
        case "deb":
            _get_package_url = get_deb_package_url
            _extract = extract_deb
        case "rpm":
            _get_package_url = get_rpm_package_url
            _extract = extract_rpm
        case _:
            raise Exit(code=1, message=f"Unknown package type: {_type}")

    package_name = get_package_name(binary, flavor)
    download_path = _download(_get_package_url(ctx, int(pipeline), package_name, arch), path)

    if extract:
        _extract(ctx, download_path, extract_dir)
