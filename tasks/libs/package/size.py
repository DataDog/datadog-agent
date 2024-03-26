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
        "agent": "opt/datadog-dogstatsd/bin/dogstatsd",
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
    elif package_os == CENTOS_OS or package_os == SUSE_OS:
        return extract_rpm_package(ctx, package_path, extract_dir)
    else:
        raise Exception(
            message=color_message(
                f"Provided OS {package_os} doesn't match any of: {DEBIAN_OS}, {CENTOS_OS}, {SUSE_OS}", "red"
            )
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
    from tasks.libs.datadog_api import create_gauge

    series = []
    with tempfile.TemporaryDirectory() as extract_dir:
        extract_package(ctx=ctx, package_os=package_os, package_path=package_path, extract_dir=extract_dir)

        package_compressed_size = os.path.getsize(package_path)

        # HACK: For uncompressed size, fall back to native Unix utilities - computing a directory size with Python
        # TODO: To make this work on other OSes, the complete directory walk would need to be implemented
        package_uncompressed_size = int(ctx.run(f"du -sB1 {extract_dir}", hide=True).stdout.split()[0])

        timestamp = int(datetime.now().timestamp())
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
            binary_size = os.path.getsize(os.path.join(extract_dir, binary_path))
            series.append(
                create_gauge(
                    "datadog.agent.binary.size",
                    timestamp,
                    binary_size,
                    tags=common_tags + [f"binary:{binary_name}"],
                )
            )

    return series
