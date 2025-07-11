import tempfile

from tasks.libs.package.size import directory_size, extract_package, file_size
from tasks.static_quality_gates.lib.gates_lib import argument_extractor, read_byte_input
from tasks.static_quality_gates.lib.package_agent_lib import check_package_size, find_package_path


def calculate_package_size(ctx, package_os, package_zip_path, package_msi_path):
    with tempfile.TemporaryDirectory() as extract_dir:
        extract_package(ctx=ctx, package_os=package_os, package_path=package_zip_path, extract_dir=extract_dir)
        package_on_wire_size = file_size(path=package_msi_path)
        package_on_disk_size = directory_size(path=extract_dir)
    return package_on_wire_size, package_on_disk_size


def entrypoint(**kwargs):
    gate_name = "static_quality_gate_agent_msi"
    arguments = argument_extractor(
        kwargs, max_on_wire_size=read_byte_input, max_on_disk_size=read_byte_input, ctx=None, metricHandler=None
    )
    ctx = arguments.ctx
    metric_handler = arguments.metricHandler
    max_on_wire_size = arguments.max_on_wire_size
    max_on_disk_size = arguments.max_on_disk_size

    metric_handler.register_gate_tags(gate_name, gate_name=gate_name, arch="amd64", os="windows")

    metric_handler.register_metric(gate_name, "max_on_wire_size", max_on_wire_size)
    metric_handler.register_metric(gate_name, "max_on_disk_size", max_on_disk_size)

    package_zip_path = find_package_path("datadog-agent", "windows", "x86_64", extension="zip")
    package_msi_path = find_package_path("datadog-agent", "windows", "x86_64", extension="msi")

    package_on_wire_size, package_on_disk_size = calculate_package_size(
        ctx, "windows", package_zip_path, package_msi_path
    )

    metric_handler.register_metric(gate_name, "current_on_wire_size", package_on_wire_size)
    metric_handler.register_metric(gate_name, "current_on_disk_size", package_on_disk_size)

    check_package_size(package_on_wire_size, package_on_disk_size, max_on_wire_size, max_on_disk_size)


def debug_entrypoint(**kwargs):
    raise NotImplementedError()
