import os

from tasks.libs.common.color import color_message
from tasks.static_quality_gates.lib.gates_lib import argument_extractor


def entrypoint(**kwargs):
    arguments = argument_extractor(kwargs, max_on_wire_size=None, max_on_disk_size=None, ctx=None, metricHandler=None)
    ctx = arguments.ctx
    metricHandler = arguments.metricHandler
    max_on_wire_size = arguments.max_on_wire_size
    max_on_disk_size = arguments.max_on_disk_size

    metricHandler.register_gate_tags(
        "static_quality_gate_docker_agent", gate_name="static_quality_gate_docker_agent", arch="x64"
    )

    metricHandler.register_metric("static_quality_gate_docker_agent", "max_on_wire_size", max_on_wire_size)
    metricHandler.register_metric("static_quality_gate_docker_agent", "max_on_disk_size", max_on_disk_size)

    pipeline_id = os.environ["CI_PIPELINE_ID"]
    commit_sha = os.environ["CI_COMMIT_SHORT_SHA"]
    arch = "amd64"
    if not pipeline_id or not commit_sha:
        raise Exception(
            color_message(
                "This gate needs to be ran from the CI environment. (Missing CI_PIPELINE_ID, CI_COMMIT_SHORT_SHA)",
                "orange",
            )
        )
    url = f"registry.ddbuild.io/ci/datadog-agent/agent:v{pipeline_id}-{commit_sha}-7-{arch}"
    image_on_wire_size = ctx.run(
        "DOCKER_CLI_EXPERIMENTAL=enabled docker manifest inspect -v "
        + url
        + " | grep size | awk -F ':' '{sum+=$NF} END {print sum}'"
    )
    image_on_wire_size = int(image_on_wire_size.stdout)
    # Pull image locally to get on disk size
    ctx.run(f"crane pull {url} output.tar")
    image_on_disk_size = ctx.run("tar -xf output.tar --to-stdout | wc -c")
    image_on_disk_size = int(image_on_disk_size.stdout)

    metricHandler.register_metric("static_quality_gate_docker_agent", "current_on_wire_size", image_on_wire_size)
    metricHandler.register_metric("static_quality_gate_docker_agent", "current_on_disk_size", image_on_disk_size)

    error_message = ""
    if image_on_wire_size > max_on_wire_size:
        error_message += color_message(
            f"Image size on wire (compressed image size) {image_on_wire_size} is higher than the maximum allowed {max_on_wire_size} by the gate !\n",
            "red",
        )
    else:
        print(
            color_message(
                f"image_on_wire_size <= max_on_wire_size, ({image_on_wire_size}) <= ({max_on_wire_size})",
                "green",
            )
        )
    if image_on_disk_size > max_on_disk_size:
        error_message += color_message(
            f"Image size on disk (uncompressed image size) {image_on_disk_size} is higher than the maximum allowed {max_on_disk_size} by the gate !\n",
            "red",
        )
    else:
        print(
            color_message(
                f"image_on_disk_size <= max_on_wire_size, ({image_on_disk_size}) <= ({max_on_disk_size})",
                "green",
            )
        )
    if error_message != "":
        raise AssertionError(error_message)
