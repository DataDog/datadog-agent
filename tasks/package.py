import os
import shlex
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
from tasks.libs.package.url import get_deb_package_url, get_docker_image_url, get_installer_oci_url, get_rpm_package_url
from tasks.libs.package.utils import (
    PackageSize,
    get_ancestor,
    get_package_name,
    list_packages,
    retrieve_package_sizes,
    upload_package_sizes,
)
from tasks.libs.pipeline.utils import get_pipeline_id


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
    download(
        ctx,
        pipeline=base_pipeline_id,
        binary=binary,
        _type=_type,
        flavor=flavor,
        arch=arch,
        path=tmpdir,
        extract_dir=base_extract_dir,
    )
    target_extract_dir = os.path.join(tmpdir, "target")
    download(
        ctx,
        pipeline=target_pipeline_id,
        binary=binary,
        _type=_type,
        flavor=flavor,
        arch=arch,
        path=tmpdir,
        extract_dir=target_extract_dir,
    )

    _diff(base_extract_dir, target_extract_dir, sort_by_size=sort_by_size)


@task(
    help={
        "pipeline": "The pipeline id to download the package from",
        "pull_request_id": "Download the package from the pull request's target (head) pipeline.",
        "binary": "The binary of the package, either agent, dogstatsd, or installer",
        "type": "The type of package: deb, rpm, docker, or oci",
        "flavor": "The flavor of the package, either empty, heroku, iot or fips",
        "arch": "The package architecture, either amd64 or arm64",
        "path": "The path to download the package to",
        "extract": "Whether to extract the package (ignored for docker/oci types)",
        "extract_dir": "The directory to extract the package to",
        "ecr_release_suffix": "ECR image repo suffix for nightly/release pipelines, e.g. '-nightly' or '-release'",
    }
)
def download(
    ctx: Context,
    pipeline: str | int | None = None,
    binary: str = "agent",
    _type: str = "deb",
    flavor: str = "",
    arch: str = "amd64",
    path: str | None = None,
    extract: bool = True,
    extract_dir: str | None = None,
    pull_request_id: str | None = None,
    ecr_release_suffix: str = "",
):
    """
    Download the package from the given pipeline.

    Exactly one of --pipeline or --pull-request-id must be provided.
    For docker/oci types, extraction is always disabled.
    The installer OCI image uses a different registry and tag scheme from agent/dogstatsd.
    """

    assert binary in ("agent", "dogstatsd", "installer"), "Unknown binary"
    assert flavor in ("", "heroku", "iot", "fips"), "Unknown flavor"
    assert arch in ("amd64", "arm64"), "Unknown architecture"

    repo = None
    if pull_request_id is not None:
        assert pipeline is None, "Only one of --pipeline or --pull-request-id may be provided"
        repo = get_gitlab_repo("DataDog/datadog-agent")
        pipeline = get_pipeline_id(ctx, repo, None, None, pull_request_id, base=False)

    assert pipeline is not None, "One of --pipeline or --pull-request-id must be provided"

    if path is None:
        path = os.getcwd()
    if extract_dir is None:
        extract_dir = path
    else:
        # If extract_dir is provided, always extract
        extract = True

    match _type:
        case "deb":
            package_name = get_package_name(binary, flavor)
            download_path = _download(get_deb_package_url(ctx, int(pipeline), package_name, arch), path)
            if extract:
                extract_deb(ctx, download_path, extract_dir)

        case "rpm":
            package_name = get_package_name(binary, flavor)
            download_path = _download(get_rpm_package_url(ctx, int(pipeline), package_name, arch), path)
            if extract:
                extract_rpm(ctx, download_path, extract_dir)

        case "docker" | "oci":
            if binary == "installer":
                image_url = get_installer_oci_url(int(pipeline))
            else:
                if repo is None:
                    repo = get_gitlab_repo("DataDog/datadog-agent")
                gitlab_pipeline = repo.pipelines.get(int(pipeline))
                commit_short_sha = gitlab_pipeline.sha[:8]
                image_url = get_docker_image_url(
                    int(pipeline), binary, flavor, arch, commit_short_sha, ecr_release_suffix
                )
            if not os.path.isdir(path):
                raise Exit(code=1, message=f"--path must be a directory for docker/oci downloads, got: {path}")
            output_path = os.path.join(path, f"{get_package_name(binary, flavor)}-{arch}.{_type}")
            crane_format = "--format=oci " if _type == "oci" else ""
            ctx.run(f"crane pull {crane_format}{shlex.quote(image_url)} {shlex.quote(output_path)}")

        case _:
            raise Exit(code=1, message=f"Unknown package type: {_type}")
