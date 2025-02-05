from tasks.libs.common.color import color_message


def calculate_image_on_disk_size(ctx, url):
    # Pull image locally to get on disk size
    ctx.run(f"crane pull {url} output.tar")
    # The downloaded image contains some metadata files and another tar.gz file. We are computing the sum of
    # these metadata files and the uncompressed size of the tar.gz inside of output.tar.
    ctx.run("tar -xf output.tar")
    image_content = ctx.run("tar -tvf output.tar | awk -F' ' '{print $3; print $6}'").stdout.splitlines()
    total_size = 0
    image_tar_gz = None
    for k, line in enumerate(image_content):
        if k % 2 == 0:
            if "tar.gz" in image_content[k + 1]:
                image_tar_gz = image_content[k + 1]
            else:
                total_size += int(line)

    if image_tar_gz:
        total_size += int(ctx.run(f"tar -xf {image_tar_gz} --to-stdout | wc -c").stdout)

    return total_size


def get_image_url_size(ctx, metric_handler, gate_name, url):
    image_on_wire_size = ctx.run(
        "DOCKER_CLI_EXPERIMENTAL=enabled docker manifest inspect -v "
        + url
        + " | grep size | awk -F ':' '{sum+=$NF} END {print sum}'"
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
