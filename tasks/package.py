import os
from datetime import datetime

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_default_branch
from tasks.libs.package.size import (
    PACKAGE_SIZE_TEMPLATE,
    _get_deb_uncompressed_size,
    _get_rpm_uncompressed_size,
    compare,
    compute_package_size_metrics,
)
from tasks.libs.package.utils import (
    PackageSize,
    display_message,
    find_package,
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
        size_message = "".join(f"{pkg_size.markdown()}\n" for pkg_size in size_table)
        if "❌" in size_message:
            decision = "❌ Failed"
        elif "⚠️" in size_message:
            decision = "⚠️ Warning"
        else:
            decision = "✅ Passed"
        display_message(ctx, ancestor, size_message, decision)
        if "Failed" in decision:
            raise Exit(code=1)


@task
def compare_size(ctx, new_package, stable_package, package_type, last_stable, threshold):
    mb = 1000000

    if package_type.endswith('deb'):
        new_package_size = _get_deb_uncompressed_size(ctx, find_package(new_package))
        stable_package_size = _get_deb_uncompressed_size(ctx, find_package(stable_package))
    else:
        new_package_size = _get_rpm_uncompressed_size(ctx, find_package(new_package))
        stable_package_size = _get_rpm_uncompressed_size(ctx, find_package(stable_package))

    threshold = int(threshold)

    diff = new_package_size - stable_package_size

    # For printing purposes
    new_package_size_mb = new_package_size / mb
    stable_package_size_mb = stable_package_size / mb
    threshold_mb = threshold / mb
    diff_mb = diff / mb

    if diff > threshold:
        print(
            color_message(
                f"""{package_type} size increase is too large:
  New package size is {new_package_size_mb:.2f}MB
  Stable package ({last_stable}) size is {stable_package_size_mb:.2f}MB
  Diff is {diff_mb:.2f}MB > {threshold_mb:.2f}MB (max allowed diff)""",
                "red",
            )
        )
        raise Exit(code=1)

    print(
        f"""{package_type} size increase is OK:
  New package size is {new_package_size_mb:.2f}MB
  Stable package ({last_stable}) size is {stable_package_size_mb:.2f}MB
  Diff is {diff_mb:.2f}MB (max allowed diff: {threshold_mb:.2f}MB)"""
    )


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
