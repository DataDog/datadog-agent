import os

from tasks.libs.common.color import color_message
from tasks.static_quality_gates.lib.gates_lib import argument_extractor


def entrypoint(**kwargs):
    arguments = argument_extractor(kwargs, max_on_wire_size=None, max_on_disk_size=None, ctx=None)
    ctx = arguments.ctx
    max_on_wire_size = arguments.max_on_wire_size
    max_on_disk_size = arguments.max_on_disk_size
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
    print(image_on_wire_size, max_on_wire_size, max_on_disk_size)
