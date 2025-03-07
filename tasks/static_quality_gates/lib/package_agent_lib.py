import tempfile

from tasks.libs.common.color import color_message
from tasks.libs.package.size import directory_size, extract_package, file_size
from tasks.static_quality_gates.lib.gates_lib import argument_extractor, find_package_path, read_byte_input


def calculate_package_size(ctx, package_os, package_path, gate_name, metric_handler):
    with tempfile.TemporaryDirectory() as extract_dir:
        extract_package(ctx=ctx, package_os=package_os, package_path=package_path, extract_dir=extract_dir)
        package_on_wire_size = file_size(path=package_path)
        package_on_disk_size = directory_size(ctx, path=extract_dir)

        metric_handler.register_metric(gate_name, "current_on_wire_size", package_on_wire_size)
        metric_handler.register_metric(gate_name, "current_on_disk_size", package_on_disk_size)
    return package_on_wire_size, package_on_disk_size


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


def generic_package_agent_quality_gate(gate_name, arch, os, flavor, **kwargs):
    arguments = argument_extractor(
        kwargs, max_on_wire_size=read_byte_input, max_on_disk_size=read_byte_input, ctx=None, metricHandler=None
    )
    ctx = arguments.ctx
    metric_handler = arguments.metricHandler
    max_on_wire_size = arguments.max_on_wire_size
    max_on_disk_size = arguments.max_on_disk_size

    metric_handler.register_gate_tags(gate_name, gate_name=gate_name, arch=arch, os=os)

    metric_handler.register_metric(gate_name, "max_on_wire_size", max_on_wire_size)
    metric_handler.register_metric(gate_name, "max_on_disk_size", max_on_disk_size)
    package_arch = arch
    if os == "centos" or os == "suse":
        if arch == "arm64":
            package_arch = "aarch64"
        elif arch == "amd64":
            package_arch = "x86_64"
        elif arch == "armhf":
            package_arch = "armv7hl"

    package_os = os
    if os == "heroku":
        package_os = "debian"

    package_path = find_package_path(flavor, package_os, package_arch)

    package_on_wire_size, package_on_disk_size = calculate_package_size(
        ctx, os, package_path, gate_name, metric_handler
    )
    check_package_size(package_on_wire_size, package_on_disk_size, max_on_wire_size, max_on_disk_size)
