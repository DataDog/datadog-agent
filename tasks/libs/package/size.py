import os
import sys
import tempfile
from datetime import datetime

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.constants import ORIGIN_CATEGORY, ORIGIN_PRODUCT, ORIGIN_SERVICE
from tasks.libs.common.git import get_default_branch
from tasks.libs.common.utils import get_metric_origin
from tasks.libs.package.utils import find_package

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

# The below template contains the relative increase threshold for each package type
PACKAGE_SIZE_TEMPLATE = {
    'amd64': {
        'datadog-agent': {'deb': int(140e6)},
        'datadog-iot-agent': {'deb': int(10e6)},
        'datadog-dogstatsd': {'deb': int(10e6)},
        'datadog-heroku-agent': {'deb': int(70e6)},
    },
    'x86_64': {
        'datadog-agent': {'rpm': int(140e6), 'suse': int(140e6)},
        'datadog-iot-agent': {'rpm': int(10e6), 'suse': int(10e6)},
        'datadog-dogstatsd': {'rpm': int(10e6), 'suse': int(10e6)},
    },
    'arm64': {
        'datadog-agent': {'deb': int(140e6)},
        'datadog-iot-agent': {'deb': int(10e6)},
        'datadog-dogstatsd': {'deb': int(10e6)},
    },
    'aarch64': {'datadog-agent': {'rpm': int(140e6)}, 'datadog-iot-agent': {'rpm': int(10e6)}},
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


def compare(ctx, package_sizes, ancestor, pkg_size):
    """
    Compare (or update, when on main branch) a package size with the ancestor package size.
    """
    current_size = _get_uncompressed_size(ctx, find_package(pkg_size.path()), pkg_size.os)
    if os.environ['CI_COMMIT_REF_NAME'] == get_default_branch():
        # On main, ancestor is the current commit, so we set the current value
        package_sizes[ancestor][pkg_size.arch][pkg_size.flavor][pkg_size.os] = current_size
        return
    previous_size = package_sizes[ancestor][pkg_size.arch][pkg_size.flavor][pkg_size.os]
    pkg_size.compare(current_size, previous_size)

    if pkg_size.ko():
        print(color_message(pkg_size.log(), Color.RED), file=sys.stderr)
    else:
        print(pkg_size.log())
    return pkg_size


def _get_uncompressed_size(ctx, package, os_name):
    if os_name == 'deb':
        return (
            int(ctx.run(f'dpkg-deb --info {package} | grep Installed-Size | cut -d : -f 2 | xargs', hide=True).stdout)
            * 1024
        )
    else:
        return int(ctx.run(f'rpm -qip {package} | grep Size | cut -d : -f 2 | xargs', hide=True).stdout)
