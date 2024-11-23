import glob
import json

from invoke import Exit, UnexpectedExit

from tasks.libs.common.color import color_message
from tasks.libs.notify.utils import AWS_S3_CP_CMD

PACKAGE_SIZE_S3_CI_BUCKET_URL = "s3://dd-ci-artefacts-build-stable/datadog-agent/package_size"


def get_package_path(glob_pattern):
    package_paths = glob.glob(glob_pattern)
    if len(package_paths) > 1:
        raise Exit(code=1, message=color_message(f"Too many files matching {glob_pattern}: {package_paths}", "red"))
    elif len(package_paths) == 0:
        raise Exit(code=1, message=color_message(f"Couldn't find any file matching {glob_pattern}", "red"))

    return package_paths[0]


def list_packages(template, parent=""):
    """
    Recursively parse the template to generate the argument list for the compare task
    """
    packs = []
    parent = parent if parent else []
    for key, value in template.items():
        if isinstance(value, dict):
            packs += list_packages(value, parent + [key])
        else:
            packs.append(parent + [key, value])
    return packs


def retrieve_package_sizes(ctx, package_size_file):
    """
    Retrieve the stored document in aws s3, or create it
    The content of the file is the following:
    {
        "c0ae34b1": {
            "timestamp": "1582230020",
            "amd64": {
                "datadog-agent": {
                    "deb": 123456,
                    "rpm": 123456,
                    "suse": 123456}}}
    """
    try:
        ctx.run(
            f"{AWS_S3_CP_CMD} {PACKAGE_SIZE_S3_CI_BUCKET_URL}/{package_size_file} {package_size_file}",
            hide=True,
        )
        with open(package_size_file) as f:
            package_sizes = json.load(f)
    except UnexpectedExit as e:
        if "404" in e.result.stderr:
            package_sizes = {}
        else:
            raise e
    return package_sizes


def upload_package_size(ctx, package_sizes: dict, package_size_file: str):
    """
    Save the package_sizes dict to a file and upload it to the CI bucket
    """
    with open(package_size_file, "w") as f:
        json.dump(package_sizes, f)
    ctx.run(
        f"{AWS_S3_CP_CMD} {package_size_file} {PACKAGE_SIZE_S3_CI_BUCKET_URL}/{package_size_file}",
        hide="stdout",
    )
