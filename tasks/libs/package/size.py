import os
import tempfile
from datetime import datetime

from invoke import Exit

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.constants import ORIGIN_CATEGORY, ORIGIN_PRODUCT, ORIGIN_SERVICE
from tasks.libs.common.git import get_common_ancestor, get_current_branch, get_default_branch
from tasks.libs.common.utils import get_metric_origin
from tasks.libs.package.utils import get_package_path

DEBIAN_OS = "debian"
CENTOS_OS = "centos"
SUSE_OS = "suse"

SCANNED_BINARIES = {
    "agent": {
        "agent": "opt/datadog-agent/bin/agent/agent",
        "process-agent": "opt/datadog-agent/embedded/bin/process-agent",
        "trace-agent": "opt/datadog-agent/embedded/bin/trace-agent",
        "system-probe": "opt/datadog-agent/embedded/bin/system-probe",
        "security-agent": "opt/datadog-agent/embedded/bin/security-agent",
    },
    "iot-agent": {
        "agent": "opt/datadog-agent/bin/agent/agent",
    },
    "dogstatsd": {
        "dogstatsd": "opt/datadog-dogstatsd/bin/dogstatsd",
    },
    "heroku-agent": {
        "agent": "opt/datadog-agent/bin/agent/agent",
        "process-agent": "opt/datadog-agent/embedded/bin/process-agent",
        "trace-agent": "opt/datadog-agent/embedded/bin/trace-agent",
    },
}

PACKAGE_SIZE_TEMPLATE = {
    'amd64': {
        'datadog-agent': {'deb': 140000000, 'rpm': 140000000, 'suse': 140000000},
        'datadog-iot-agent': {'deb': 10000000, 'rpm': 10000000, 'suse': 10000000},
        'datadog-dogstatsd': {'deb': 10000000, 'rpm': 10000000, 'suse': 10000000},
        'datadog-heroku-agent': {'deb': 70000000},
    },
    'arm64': {
        'datadog-agent': {'deb': 140000000},
        'datadog-iot-agent': {'deb': 10000000},
        'datadog-dogstatsd': {'deb': 10000000},
    },
    'aarch64': {'datadog-agent': {'rpm': 140000000}, 'datadog-iot-agent': {'rpm': 10000000}},
}


def extract_deb_package(ctx, package_path, extract_dir):
    ctx.run(f"dpkg -x {package_path} {extract_dir} > /dev/null")


def extract_rpm_package(ctx, package_path, extract_dir):
    with ctx.cd(extract_dir):
        ctx.run(f"rpm2cpio {package_path} | cpio -idm > /dev/null")


def extract_package(ctx, package_os, package_path, extract_dir):
    if package_os == DEBIAN_OS:
        return extract_deb_package(ctx, package_path, extract_dir)
    elif package_os in (CENTOS_OS, SUSE_OS):
        return extract_rpm_package(ctx, package_path, extract_dir)
    else:
        raise ValueError(
            message=color_message(
                f"Provided OS {package_os} doesn't match any of: {DEBIAN_OS}, {CENTOS_OS}, {SUSE_OS}", "red"
            )
        )


def file_size(path):
    return os.path.getsize(path)


def directory_size(ctx, path):
    # HACK: For uncompressed size, fall back to native Unix utilities - computing a directory size with Python
    # TODO: To make this work on other OSes, the complete directory walk would need to be implemented
    return int(ctx.run(f"du -sB1 {path}", hide=True).stdout.split()[0])


def compute_package_size_metrics(
    ctx,
    flavor: str,
    package_os: str,
    package_path: str,
    major_version: str,
    git_ref: str,
    bucket_branch: str,
    arch: str,
):
    """
    Takes a flavor, os, and package path, retrieves information about the size of the package and
    of interesting binaries inside, and returns gauge metrics to report them to Datadog.
    """

    from tasks.libs.common.datadog_api import create_gauge

    if flavor not in SCANNED_BINARIES.keys():
        raise ValueError(f"'{flavor}' is not part of the accepted flavors: {', '.join(SCANNED_BINARIES.keys())}")

    series = []
    with tempfile.TemporaryDirectory() as extract_dir:
        extract_package(ctx=ctx, package_os=package_os, package_path=package_path, extract_dir=extract_dir)

        package_compressed_size = file_size(path=package_path)
        package_uncompressed_size = directory_size(ctx, path=extract_dir)

        timestamp = int(datetime.utcnow().timestamp())
        common_tags = [
            f"os:{package_os}",
            f"package:datadog-{flavor}",
            f"agent:{major_version}",
            f"git_ref:{git_ref}",
            f"bucket_branch:{bucket_branch}",
            f"arch:{arch}",
        ]
        series.append(
            create_gauge(
                "datadog.agent.compressed_package.size",
                timestamp,
                package_compressed_size,
                tags=common_tags,
                metric_origin=get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE),
            ),
        )
        series.append(
            create_gauge(
                "datadog.agent.package.size",
                timestamp,
                package_uncompressed_size,
                tags=common_tags,
                metric_origin=get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE),
            ),
        )

        for binary_name, binary_path in SCANNED_BINARIES[flavor].items():
            binary_size = file_size(os.path.join(extract_dir, binary_path))
            series.append(
                create_gauge(
                    "datadog.agent.binary.size",
                    timestamp,
                    binary_size,
                    tags=common_tags + [f"bin:{binary_name}"],
                    metric_origin=get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE),
                ),
            )

    return series


def compare(ctx, package_sizes, arch, flavor, os_name, threshold):
    """
    Compare (or update) a package size with the ancestor package size.
    """
    mb = 1000000
    if os_name == 'suse':
        dir = os.environ['OMNIBUS_PACKAGE_DIR_SUSE']
        path = f'{dir}/{flavor}_7*_{arch}.rpm'
    else:
        dir = os.environ['OMNIBUS_PACKAGE_DIR']
        path = f'{dir}/{flavor}_7*_{arch}.{os_name}'
    package_size = _get_uncompressed_size(ctx, get_package_path(path), os_name)
    branch = get_current_branch(ctx)
    ancestor = get_common_ancestor(ctx, branch)
    if branch == get_default_branch():
        package_sizes[ancestor][arch][flavor][os_name] = package_size
        return
    previous_size = get_previous_size(package_sizes, ancestor, arch, flavor, os_name)
    diff = package_size - previous_size

    # For printing purposes
    new_package_size_mb = package_size / mb
    stable_package_size_mb = previous_size / mb
    threshold_mb = threshold / mb
    diff_mb = diff / mb
    message = f"""{flavor}-{arch}-{os_name} size increase is OK:
  New package size is {new_package_size_mb:.2f}MB
  Ancestor package ({ancestor}) size is {stable_package_size_mb:.2f}MB
  Diff is {diff_mb:.2f}MB (max allowed diff: {threshold_mb:.2f}MB)"""

    if diff > threshold:
        print(color_message(message.replace('OK', 'too large'), Color.RED))
        raise Exit(code=1)

    print(message)


def get_previous_size(package_sizes, ancestor, arch, flavor, os_name):
    """
    Get the size of the package for the given ancestor, or the earliest ancestor if the given ancestor is not found.
    """
    if ancestor in package_sizes:
        commit = ancestor
    else:
        commit = min(package_sizes, key=lambda x: package_sizes[x]['timestamp'])
    return package_sizes[commit][arch][flavor][os_name]


def _get_uncompressed_size(ctx, package, os_name):
    if os_name == 'deb':
        return _get_deb_uncompressed_size(ctx, package)
    else:
        return _get_rpm_uncompressed_size(ctx, package)


def _get_deb_uncompressed_size(ctx, package):
    # the size returned by dpkg is a number of bytes divided by 1024
    # so we multiply it back to get the same unit as RPM or stat
    return int(ctx.run(f'dpkg-deb --info {package} | grep Installed-Size | cut -d : -f 2 | xargs').stdout) * 1024


def _get_rpm_uncompressed_size(ctx, package):
    return int(ctx.run(f'rpm -qip {package} | grep Size | cut -d : -f 2 | xargs').stdout)
