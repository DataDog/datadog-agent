import os

from tasks.libs.common.color import color_message
from tasks.static_quality_gates.lib.docker_agent_lib import check_image_size, get_image_url_size
from tasks.static_quality_gates.lib.gates_lib import argument_extractor, read_byte_input


def entrypoint(**kwargs):
    arguments = argument_extractor(
        kwargs, max_on_wire_size=read_byte_input, max_on_disk_size=read_byte_input, ctx=None, metricHandler=None
    )
    ctx = arguments.ctx
    metric_handler = arguments.metricHandler
    max_on_wire_size = arguments.max_on_wire_size
    max_on_disk_size = arguments.max_on_disk_size
    gate_name = "static_quality_gate_docker_agent_amd64"

    metric_handler.register_gate_tags(gate_name, gate_name=gate_name, arch="x64", os="docker")

    metric_handler.register_metric(gate_name, "max_on_wire_size", max_on_wire_size)
    metric_handler.register_metric(gate_name, "max_on_disk_size", max_on_disk_size)

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
    # Fetch the on wire and on disk size of the image from the url
    image_on_wire_size, image_on_disk_size = get_image_url_size(ctx, metric_handler, gate_name, url)
    # Check if the docker image is within acceptable bounds
    check_image_size(image_on_wire_size, image_on_disk_size, max_on_wire_size, max_on_disk_size)
