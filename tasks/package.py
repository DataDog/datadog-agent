import os
import tempfile
import xml.etree.ElementTree as ET
from datetime import datetime

import requests
from gitlab.v4.objects import Project, ProjectPipeline
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.download import download as _download
from tasks.libs.common.git import get_default_branch
from tasks.libs.package.size import (
    PACKAGE_SIZE_TEMPLATE,
    compare,
    compute_package_size_metrics,
)
from tasks.libs.package.utils import (
    PackageSize,
    display_message,
    get_ancestor,
    list_packages,
    retrieve_package_sizes,
    upload_package_sizes,
)


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
    else:
        size_table.sort(key=lambda x: (-x.diff, x.flavor, x.arch_name()))
        size_message = "".join(f"{pkg_size.markdown()}\n" for pkg_size in size_table if pkg_size.diff >= 0)
        reduction_size_message = "".join(f"{pkg_size.markdown()}\n" for pkg_size in size_table if pkg_size.diff < 0)
        if "❌" in size_message:
            decision = "❌ Failed"
        elif "⚠️" in size_message:
            decision = "⚠️ Warning"
        else:
            decision = "✅ Passed"
        # Try to display the message on the PR when a PR exists
        if os.environ.get("CI_COMMIT_BRANCH"):
            try:
                display_message(ctx, ancestor, size_message, reduction_size_message, decision)
            # PR commenter asserts on the numbers of PR's, this will raise if there's no PR
            except AssertionError as exc:
                print(f"Got `{exc}` while trying to comment on PR, we'll assume that this is not a PR.")
        if "Failed" in decision:
            raise Exit(code=0)


@task
def send_size(
    ctx,
    flavor: str,
    package_os: str,
    package_path: str,
    major_version: str,
    git_ref: str,
    bucket_branch: str,
    arch: str,
    send_series: bool = True,
):
    """
    For a provided package path, os and flavor, retrieves size information on the package and its included
    Agent binaries, prints them, and sends them to Datadog.

    The --major-version, --git-ref, --bucket-branch, and --arch parameters are used to add tags to the metrics.

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
        major_version=major_version,
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
):
    """
    Diff the content of the given package.

    Exactly one of --base-ref, --base-pipeline, or --pull-request-id must be provided.
    Exactly one of --target-ref, --target-pipeline, or --pull-request-id must be provided.
    """

    repo = get_gitlab_repo("DataDog/datadog-agent")
    base_pipeline_id = _get_pipeline_id(ctx, repo, base_ref, base_pipeline, pull_request_id, base=True)
    target_pipeline_id = _get_pipeline_id(ctx, repo, target_ref, target_pipeline, pull_request_id, base=False)
    print(f"Comparing package from pipeline {base_pipeline_id} with pipeline {target_pipeline_id}")

    print()

    tmpdir = tempfile.mkdtemp()
    print(f"Artifacts will be downloaded to {tmpdir}")

    base_extract_dir = os.path.join(tmpdir, "base")
    download(ctx, base_pipeline_id, binary, _type, flavor, arch, tmpdir, extract_dir=base_extract_dir)
    target_extract_dir = os.path.join(tmpdir, "target")
    download(ctx, target_pipeline_id, binary, _type, flavor, arch, tmpdir, extract_dir=target_extract_dir)

    _diff(base_extract_dir, target_extract_dir)


@task(
    help={
        "pipeline": "The pipeline id to download the package from",
        "binary": "The binary of the package, either agent or dogstatsd",
        "type": "The type of package, only deb is supported for now",
        "flavor": "The flavor of the package, either empty, heroku, iot or fips",
        "arch": "The package architecture, either amd64 or arm64",
        "path": "The path to download the package to",
        "extract": "Whether to extract the package",
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
            _get_package_url = _get_deb_package_url
            _extract = _extract_deb
        case "rpm":
            _get_package_url = _get_rpm_package_url
            _extract = _extract_rpm
        case _:
            raise Exit(code=1, message=f"Unknown package type: {_type}")

    package_name = _get_package_name(binary, flavor)
    download_path = _download(_get_package_url(ctx, int(pipeline), package_name, arch), path)

    if extract:
        _extract(ctx, download_path, extract_dir)


def _get_package_name(binary: str, flavor: str):
    package_name = "datadog-"
    if flavor:
        package_name += f"{flavor}-"
    package_name += binary
    return package_name


def _extract_deb(ctx: Context, deb_path: str, extract_path: str):
    os.makedirs(extract_path)
    ctx.run(f"tar xf {deb_path} -C {extract_path}")
    with ctx.cd(extract_path):
        ctx.run("tar xf data.tar.xz")
        ctx.run("rm data.tar.xz")


def _extract_rpm(ctx: Context, rpm_path: str, extract_path: str):
    os.makedirs(extract_path)
    ctx.run(f"tar xf {rpm_path} -C {extract_path}")


def _get_rpm_package_url(ctx: Context, pipeline_id: int, package_name: str, arch: str):
    base_url = "https://yumtesting.datad0g.com"
    arch2 = "x86_64" if arch == "amd64" else "aarch64"
    packages_url = f"{base_url}/testing/pipeline-{pipeline_id}-a7/7/{arch2}"

    repomd_url = f"{packages_url}/repodata/repomd.xml"
    response = requests.get(repomd_url, timeout=None)
    response.raise_for_status()
    repomd = ET.fromstring(response.text)

    primary = next((data for data in repomd.findall('.//{*}data') if data.get('type') == 'primary'), None)
    assert primary is not None, f"Could not find primary data in {repomd_url}"
    location = primary.find('{*}location')
    assert location is not None, f"Could not find location for primary data in {repomd_url}"

    filename = tempfile.mktemp()
    primary_url = f"{packages_url}/{location.get('href')}"
    _download(primary_url, filename)
    res = ctx.run(f"gunzip --stdout {filename}", hide=True)
    assert res

    primary = ET.fromstring(res.stdout.strip())
    for package in primary.findall('.//{*}package'):
        if package.get('type') != 'rpm':
            continue
        name = package.find('{*}name')
        if name is None or name.text != package_name:
            continue
        location = package.find('{*}location')
        assert location is not None, f"Could not find location for {package_name} in {primary_url}"
        return f"{packages_url}/{location.get('href')}"
    raise Exit(code=1, message=f"Could not find package {package_name} in {primary_url}")


def _get_deb_package_url(_: Context, pipeline_id: int, package_name: str, arch: str):
    arch2 = arch
    if arch == "amd64":
        arch2 = "x86_64"

    base_url = "https://apttesting.datad0g.com"
    packages_url = f"{base_url}/dists/pipeline-{pipeline_id}-a7-{arch2}/7/binary-{arch}/Packages"

    filename = _deb_get_filename_for_package(packages_url, package_name)
    return f"{base_url}/{filename}"


def _deb_get_filename_for_package(packages_url: str, target_package_name: str) -> str:
    response = requests.get(packages_url, timeout=None)
    response.raise_for_status()

    packages = [
        f"Package:{content}" if not content.startswith("Package:") else content
        for content in response.text.split("\nPackage:")
    ]

    for package in packages:
        package_name = None
        package_filename = None
        for line in package.split('\n'):
            match line.split(': ')[0]:
                case "Package":
                    package_name = line.split(': ', 1)[1]
                    continue
                case "Filename":
                    package_filename = line.split(': ', 1)[1]
                    continue

        if target_package_name == package_name:
            if package_filename is None:
                raise Exit(code=1, message=f"Could not find filename for {target_package_name} in {packages_url}")
            return package_filename

    raise Exit(code=1, message=f"Could not find filename for {target_package_name} in {packages_url}")


def _get_pipeline_id(
    ctx: Context, repo: Project, ref: str | None, pipeline: str | None, pull_request_id: str | None, base: bool
) -> int:
    nargs = int(ref is not None) + int(pipeline is not None) + int(pull_request_id is not None)
    assert nargs == 1, "Exactly one of commit, pipeline or pull_request_id must be provided"

    if pipeline is not None:
        return int(pipeline)

    if ref is not None:
        return _get_pipeline_from_ref(repo, ref).get_id()

    assert pull_request_id is not None

    gh = GithubAPI()
    pr = gh.get_pr(int(pull_request_id))
    if base:
        res = ctx.run(f"git merge-base {pr.base.ref} {pr.head.ref}", hide=True)
        assert res
        base_ref = res.stdout.strip()
        print(f"Base ref is {pr.base.ref}, merge-base is {base_ref}")
        return _get_pipeline_from_ref(repo, base_ref).get_id()

    print(f"Head ref is {pr.head.ref}")
    return _get_pipeline_from_ref(repo, pr.head.ref).get_id()


def _get_pipeline_from_ref(repo: Project, ref: str) -> ProjectPipeline:
    # Get last updated pipeline
    print(f"Getting pipeline for {ref}...")

    pipelines = repo.pipelines.list(ref=ref, per_page=1, order_by='updated_at', get_all=False)
    if len(pipelines) != 0:
        return pipelines[0]

    pipelines = repo.pipelines.list(sha=ref, per_page=1, order_by='updated_at', get_all=False)
    if len(pipelines) != 0:
        return pipelines[0]

    print(f"No pipelines found for {ref}")
    raise Exit(code=1)


def _diff(dir1: str, dir2: str):
    from binary import convert_units

    seen = set()
    dir1not2 = []

    for dirpath, _, filenames in os.walk(dir1):
        for filename in filenames:
            dir1filepath = os.path.join(dirpath, filename)
            relfilepath = dir1filepath.removeprefix(dir1)
            dir2filepath = dir2 + relfilepath

            if not os.path.exists(dir2filepath) and not os.path.islink(dir2filepath):
                dir1not2.append(relfilepath)
                continue

            seen.add(relfilepath)
            s1 = os.stat(dir1filepath, follow_symlinks=False)
            s2 = os.stat(dir2filepath, follow_symlinks=False)
            if s1.st_size != s2.st_size:
                diff = s2.st_size - s1.st_size
                sign = "+" if diff > 0 else "-"
                amount, unit = convert_units(abs(diff))
                amount = round(amount, 2)
                print(f"Size mismatch: {relfilepath} {s1.st_size} vs {s2.st_size} ({sign}{amount}{unit})")

    print()

    if dir1not2:
        print(f"Files in {dir1} but not in {dir2}:")
        for filepath in dir1not2:
            print(filepath)

        print()

    header = False
    for dirpath, _, filenames in os.walk(dir2):
        for filename in filenames:
            dir2filepath = os.path.join(dirpath, filename)
            relfilepath = dir2filepath.removeprefix(dir2)
            if relfilepath not in seen:
                if not header:
                    print(f"Files in {dir2} but not in {dir1}:")
                    header = True
                print(relfilepath)
