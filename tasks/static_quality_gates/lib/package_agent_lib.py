import os
import tempfile

from gitlab.v4.objects import ProjectPipeline, ProjectPipelineJob
from invoke import Exit

from tasks.debugging.dump import download_job_artifacts
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.color import color_message
from tasks.libs.common.diff import diff as folder_content_diff
from tasks.libs.common.git import get_common_ancestor
from tasks.libs.package.size import directory_size, extract_package, file_size
from tasks.static_quality_gates.lib.gates_lib import argument_extractor, find_package_path, read_byte_input


def calculate_package_size(ctx, package_os, package_path, gate_name, metric_handler):
    with tempfile.TemporaryDirectory() as extract_dir:
        extract_package(ctx=ctx, package_os=package_os, package_path=package_path, extract_dir=extract_dir)
        package_on_wire_size = file_size(path=package_path)
        package_on_disk_size = directory_size(path=extract_dir)

        metric_handler.register_metric(gate_name, "current_on_wire_size", package_on_wire_size)
        metric_handler.register_metric(gate_name, "current_on_disk_size", package_on_disk_size)
    return package_on_wire_size, package_on_disk_size


def format_package_os_and_arch(arch, sys_os):
    package_arch = arch
    if sys_os == "centos" or sys_os == "suse":
        if arch == "arm64":
            package_arch = "aarch64"
        elif arch == "amd64":
            package_arch = "x86_64"
        elif arch == "armhf":
            package_arch = "armv7hl"

    package_os = sys_os
    if sys_os == "heroku":
        package_os = "debian"
    return package_arch, package_os


def check_package_size(package_on_wire_size, package_on_disk_size, max_on_wire_size, max_on_disk_size):
    error_message = ""
    if package_on_wire_size > max_on_wire_size:
        err_msg = f"Package size on wire (compressed package size) {package_on_wire_size} is higher than the maximum allowed {max_on_wire_size} by the gate !\n"
        print(color_message(err_msg, "red"))
        error_message += err_msg
    else:
        print(
            color_message(
                f"package_on_wire_size <= max_on_wire_size, ({package_on_wire_size}) <= ({max_on_wire_size})",
                "green",
            )
        )
    if package_on_disk_size > max_on_disk_size:
        err_msg = f"Package size on disk (uncompressed package size) {package_on_disk_size} is higher than the maximum allowed {max_on_disk_size} by the gate !\n"
        print(color_message(err_msg, "red"))
        error_message += err_msg
    else:
        print(
            color_message(
                f"package_on_disk_size <= max_on_disk_size, ({package_on_disk_size}) <= ({max_on_disk_size})",
                "green",
            )
        )
    if error_message != "":
        raise AssertionError(error_message)


def generic_package_agent_quality_gate(gate_name, arch, sys_os, flavor, **kwargs):
    arguments = argument_extractor(
        kwargs, max_on_wire_size=read_byte_input, max_on_disk_size=read_byte_input, ctx=None, metricHandler=None
    )
    ctx = arguments.ctx
    metric_handler = arguments.metricHandler
    max_on_wire_size = arguments.max_on_wire_size
    max_on_disk_size = arguments.max_on_disk_size

    metric_handler.register_gate_tags(gate_name, gate_name=gate_name, arch=arch, os=sys_os)

    metric_handler.register_metric(gate_name, "max_on_wire_size", max_on_wire_size)
    metric_handler.register_metric(gate_name, "max_on_disk_size", max_on_disk_size)
    package_arch, package_os = format_package_os_and_arch(arch, sys_os)

    package_path = find_package_path(flavor, package_os, package_arch)

    package_on_wire_size, package_on_disk_size = calculate_package_size(
        ctx, sys_os, package_path, gate_name, metric_handler
    )
    check_package_size(package_on_wire_size, package_on_disk_size, max_on_wire_size, max_on_disk_size)


def download_packages(sha, download_dir, build_job_name):
    # Fetch the ancestor's build_job object with the gitlab API
    repo = get_gitlab_repo("DataDog/datadog-agent")
    pipeline_list = repo.pipelines.list(sha=sha)
    if not len(pipeline_list):
        raise Exit(code=1, message="Ancestor commit has no pipeline attached.")
    ancestor_pipeline: ProjectPipeline = pipeline_list[0]
    ancestor_job: ProjectPipelineJob = next(
        filter(lambda job: job.name == build_job_name, ancestor_pipeline.jobs.list(iterator=True))
    )
    # Download & extract the artifact from the build_job
    download_job_artifacts(repo, ancestor_job.get_id(), download_dir)


def debug_package_size(ctx, package_os, package_path, ancestor_package_path):
    with (
        tempfile.TemporaryDirectory() as current_pipeline_extract_dir,
        tempfile.TemporaryDirectory() as ancestor_pipeline_extract_dir,
    ):
        # extract both packages
        extract_package(
            ctx=ctx, package_os=package_os, package_path=package_path, extract_dir=current_pipeline_extract_dir
        )
        extract_package(
            ctx=ctx,
            package_os=package_os,
            package_path=ancestor_package_path,
            extract_dir=ancestor_pipeline_extract_dir,
        )
        # Compare both packages content
        folder_content_diff(current_pipeline_extract_dir, ancestor_pipeline_extract_dir)


def generic_debug_package_agent_quality_gate(arch, sys_os, flavor, **kwargs):
    arguments = argument_extractor(kwargs, ctx=None, build_job_name=None)
    ctx = arguments.ctx
    build_job_name = arguments.build_job_name

    package_arch, package_os = format_package_os_and_arch(arch, sys_os)

    with tempfile.TemporaryDirectory() as ancestor_download_dir, tempfile.TemporaryDirectory() as current_download_dir:
        ancestor_sha = get_common_ancestor(ctx, "HEAD")
        download_packages(ancestor_sha, ancestor_download_dir, build_job_name)
        download_packages(os.environ.get("CI_COMMIT_SHA"), current_download_dir, build_job_name)

        # Find the current package path from its download directory
        os.environ['OMNIBUS_PACKAGE_DIR'] = f"{current_download_dir}/omnibus/pkg"
        package_path = find_package_path(flavor, package_os, package_arch)
        # Find the ancestor package path from its download directory
        os.environ['OMNIBUS_PACKAGE_DIR'] = f"{ancestor_download_dir}/omnibus/pkg"
        ancestor_package_path = find_package_path(flavor, package_os, package_arch)

        debug_package_size(ctx, sys_os, package_path, ancestor_package_path)
