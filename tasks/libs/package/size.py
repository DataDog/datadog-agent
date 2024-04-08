import os
import tempfile
from datetime import datetime

from tasks.libs.common.color import color_message

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
            )
        )
        series.append(
            create_gauge(
                "datadog.agent.package.size",
                timestamp,
                package_uncompressed_size,
                tags=common_tags,
            )
        )

        for binary_name, binary_path in SCANNED_BINARIES[flavor].items():
            binary_size = file_size(os.path.join(extract_dir, binary_path))
            series.append(
                create_gauge(
                    "datadog.agent.binary.size",
                    timestamp,
                    binary_size,
                    tags=common_tags + [f"bin:{binary_name}"],
                )
            )

    return series
