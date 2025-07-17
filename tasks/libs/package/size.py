import os
import sys
import tempfile
from datetime import datetime
from pathlib import Path

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.constants import ORIGIN_CATEGORY, ORIGIN_PRODUCT, ORIGIN_SERVICE
from tasks.libs.common.git import get_default_branch
from tasks.libs.common.utils import get_metric_origin
from tasks.libs.package.utils import find_package

DEBIAN_OS = "debian"
HEROKU_OS = "heroku"
CENTOS_OS = "centos"
SUSE_OS = "suse"
WINDOWS_OS = "windows"
MAC_OS = "darwin"

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
        "trace-agent": "opt/datadog-agent/embedded/bin/trace-agent",
    },
}

# The below template contains the relative increase threshold for each package type
PACKAGE_SIZE_TEMPLATE = {
    'amd64': {
        'datadog-agent': {'deb': int(5e5)},
        'datadog-iot-agent': {'deb': int(5e5)},
        'datadog-dogstatsd': {'deb': int(5e5)},
        'datadog-heroku-agent': {'deb': int(5e5)},
    },
    'x86_64': {
        'datadog-agent': {'rpm': int(5e5), 'suse': int(5e5)},
        'datadog-iot-agent': {'rpm': int(5e5), 'suse': int(5e5)},
        'datadog-dogstatsd': {'rpm': int(5e5), 'suse': int(5e5)},
    },
    'arm64': {
        'datadog-agent': {'deb': int(5e5)},
        'datadog-iot-agent': {'deb': int(5e5)},
        'datadog-dogstatsd': {'deb': int(5e5)},
    },
    'aarch64': {'datadog-agent': {'rpm': int(5e5)}, 'datadog-iot-agent': {'rpm': int(5e5)}},
}


class InfraError(Exception):
    pass


def extract_deb_package(ctx, package_path, extract_dir):
    ctx.run(f"dpkg -x {package_path} {extract_dir} > /dev/null")


def extract_rpm_package(ctx, package_path, extract_dir):
    log_dir = os.environ.get("CI_PROJECT_DIR", None)
    if log_dir is None:
        log_dir = "/tmp"
    with ctx.cd(extract_dir):
        out = ctx.run(f"rpm2cpio {package_path} | cpio -idm > {log_dir}/extract_rpm_package_report", warn=True)
        if out.exited == 2:
            raise InfraError("RPM archive extraction failed ! retrying...(infra flake)")


def extract_zip_archive(ctx, package_path, extract_dir):
    with ctx.cd(extract_dir):
        ctx.run(f"unzip {package_path}", hide=True)


def extract_dmg_archive(ctx, package_path, extract_dir):
    with ctx.cd(extract_dir):
        ctx.run(f"dmg2img {package_path} -o dmg_image.img")
        ctx.run("7z x dmg_image.img")
        ctx.run("mkdir ./extracted_pkg")
        package_path_pkg_format = os.path.basename(package_path).replace("dmg", "pkg")
        ctx.run(f"xar -xf ./Agent/{package_path_pkg_format} -C ./extracted_pkg")
        ctx.run("mkdir image_content")
        with ctx.cd("image_content"):
            ctx.run("cat ../extracted_pkg/datadog-agent-core.pkg/Payload | gunzip -d | cpio -i")


def extract_package(ctx, package_os, package_path, extract_dir):
    if package_os in (DEBIAN_OS, HEROKU_OS):
        return extract_deb_package(ctx, package_path, extract_dir)
    elif package_os in (CENTOS_OS, SUSE_OS):
        return extract_rpm_package(ctx, package_path, extract_dir)
    elif package_os == WINDOWS_OS:
        return extract_zip_archive(ctx, package_path, extract_dir)
    elif package_os == MAC_OS:
        return extract_dmg_archive(ctx, package_path, extract_dir)
    else:
        raise ValueError(
            message=color_message(
                f"Provided OS {package_os} doesn't match any of: {DEBIAN_OS}, {CENTOS_OS}, {SUSE_OS}", "red"
            )
        )


def file_size(path):
    return os.path.getsize(path)


def directory_size(path):
    """Compute the size of a directory as the sum of all the files inside (recursively)"""
    return sum(
        sum((dirpath / basename).lstat().st_size for basename in filenames)
        for dirpath, _, filenames in Path(path).walk()
    )


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
        package_uncompressed_size = directory_size(path=extract_dir)

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
