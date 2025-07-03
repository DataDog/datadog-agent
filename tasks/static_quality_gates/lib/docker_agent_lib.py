import os
import sys

from tasks.libs.common.color import color_message
from tasks.libs.package.size import InfraError
from tasks.static_quality_gates.lib.gates_lib import argument_extractor, read_byte_input


def calculate_image_on_disk_size(ctx, url):
    # Pull image locally to get on disk size
    crane_output = ctx.run(f"crane pull {url} output.tar", warn=True)
    if crane_output.exited != 0:
        raise InfraError(f"Crane pull failed to retrieve {url}. Retrying... (infra flake)")
    # The downloaded image contains some metadata files and another tar.gz file. We are computing the sum of
    # these metadata files and the uncompressed size of the tar.gz inside of output.tar.
    ctx.run("tar -xf output.tar")
    image_content = ctx.run("tar -tvf output.tar | awk -F' ' '{print $3; print $6}'", hide=True).stdout.splitlines()
    total_size = 0
    image_tar_gz = []
    print("Image on disk content :")
    for k, line in enumerate(image_content):
        if k % 2 == 0:
            if "tar.gz" in image_content[k + 1]:
                image_tar_gz.append(image_content[k + 1])
            else:
                total_size += int(line)
        else:
            print(f"  - {line}")
    if image_tar_gz:
        for image in image_tar_gz:
            total_size += int(ctx.run(f"tar -xf {image} --to-stdout | wc -c", hide=True).stdout)
    else:
        print(color_message("[WARN] No tar.gz file found inside of the image", "orange"), file=sys.stderr)

    return total_size


def get_image_url_size(ctx, metric_handler, gate_name, url):
    image_on_wire_size = ctx.run(
        "DOCKER_CLI_EXPERIMENTAL=enabled docker manifest inspect -v "
        + url
        + " | grep size | awk -F ':' '{sum+=$NF} END {printf(\"%d\",sum)}'",
        hide=True,
    )
    image_on_wire_size = int(image_on_wire_size.stdout)
    # Calculate image on disk size
    image_on_disk_size = calculate_image_on_disk_size(ctx, url)

    metric_handler.register_metric(gate_name, "current_on_wire_size", image_on_wire_size)
    metric_handler.register_metric(gate_name, "current_on_disk_size", image_on_disk_size)

    return image_on_wire_size, image_on_disk_size


def check_image_size(image_on_wire_size, image_on_disk_size, max_on_wire_size, max_on_disk_size):
    error_message = ""
    if image_on_wire_size > max_on_wire_size:
        err_msg = f"Image size on wire (compressed image size) {image_on_wire_size} is higher than the maximum allowed {max_on_wire_size} by the gate !\n"
        print(color_message(err_msg, "red"))
        error_message += err_msg
    else:
        print(
            color_message(
                f"image_on_wire_size <= max_on_wire_size, ({image_on_wire_size}) <= ({max_on_wire_size})",
                "green",
            )
        )
    if image_on_disk_size > max_on_disk_size:
        err_msg = f"Image size on disk (uncompressed image size) {image_on_disk_size} is higher than the maximum allowed {max_on_disk_size} by the gate !\n"
        print(color_message(err_msg, "red"))
        error_message += err_msg
    else:
        print(
            color_message(
                f"image_on_disk_size <= max_on_disk_size, ({image_on_disk_size}) <= ({max_on_disk_size})",
                "green",
            )
        )
    if error_message != "":
        raise AssertionError(error_message)


def generic_docker_agent_quality_gate(gate_name, arch, jmx=False, flavor="agent", image_suffix="", **kwargs):
    arguments = argument_extractor(
        kwargs,
        max_on_wire_size=read_byte_input,
        max_on_disk_size=read_byte_input,
        ctx=None,
        metricHandler=None,
        nightly=None,
    )
    ctx = arguments.ctx
    metric_handler = arguments.metricHandler
    max_on_wire_size = arguments.max_on_wire_size
    max_on_disk_size = arguments.max_on_disk_size
    is_nightly_run = arguments.nightly

    metric_handler.register_gate_tags(gate_name, gate_name=gate_name, arch=arch, os="docker")

    metric_handler.register_metric(gate_name, "max_on_wire_size", max_on_wire_size)
    metric_handler.register_metric(gate_name, "max_on_disk_size", max_on_disk_size)

    pipeline_id = os.environ["CI_PIPELINE_ID"]
    commit_sha = os.environ["CI_COMMIT_SHORT_SHA"]
    if not pipeline_id or not commit_sha:
        raise Exception(
            color_message(
                "This gate needs to be ran from the CI environment. (Missing CI_PIPELINE_ID, CI_COMMIT_SHORT_SHA)",
                "orange",
            )
        )
    image_suffixes = (
        ("-7" if flavor == "agent" else "") + ("-jmx" if jmx else "") + (image_suffix if image_suffix else "")
    )
    if flavor != "dogstatsd" and is_nightly_run:
        flavor += "-nightly"
    url = f"registry.ddbuild.io/ci/datadog-agent/{flavor}:v{pipeline_id}-{commit_sha}{image_suffixes}-{arch}"
    # Fetch the on wire and on disk size of the image from the url
    image_on_wire_size, image_on_disk_size = get_image_url_size(ctx, metric_handler, gate_name, url)
    # Check if the docker image is within acceptable bounds
    check_image_size(image_on_wire_size, image_on_disk_size, max_on_wire_size, max_on_disk_size)
