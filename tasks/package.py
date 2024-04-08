import glob
import os

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.libs.package.size import compute_package_size_metrics


def get_package_path(glob_pattern):
    package_paths = glob.glob(glob_pattern)
    if len(package_paths) > 1:
        raise Exit(code=1, message=color_message(f"Too many files matching {glob_pattern}: {package_paths}", "red"))
    elif len(package_paths) == 0:
        raise Exit(code=1, message=color_message(f"Couldn't find any file matching {glob_pattern}", "red"))

    return package_paths[0]


def _get_deb_uncompressed_size(ctx, package):
    # the size returned by dpkg is a number of bytes divided by 1024
    # so we multiply it back to get the same unit as RPM or stat
    return int(ctx.run(f'dpkg-deb --info {package} | grep Installed-Size | cut -d : -f 2 | xargs').stdout) * 1024


def _get_rpm_uncompressed_size(ctx, package):
    return int(ctx.run(f'rpm -qip {package} | grep Size | cut -d : -f 2 | xargs').stdout)


@task
def compare_size(ctx, new_package, stable_package, package_type, last_stable, threshold):
    mb = 1000000

    if package_type.endswith('deb'):
        new_package_size = _get_deb_uncompressed_size(ctx, get_package_path(new_package))
        stable_package_size = _get_deb_uncompressed_size(ctx, get_package_path(stable_package))
    else:
        new_package_size = _get_rpm_uncompressed_size(ctx, get_package_path(new_package))
        stable_package_size = _get_rpm_uncompressed_size(ctx, get_package_path(stable_package))

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
