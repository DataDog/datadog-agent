import os
from datetime import datetime

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message
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
            raise Exit(code=1)


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
