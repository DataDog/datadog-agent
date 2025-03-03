import glob
import json
import os

from invoke import Exit, UnexpectedExit

from tasks.github_tasks import pr_commenter
from tasks.libs.common.color import color_message
from tasks.libs.common.git import get_common_ancestor
from tasks.libs.notify.utils import AWS_S3_CP_CMD

PACKAGE_SIZE_S3_CI_BUCKET_URL = "s3://dd-ci-artefacts-build-stable/datadog-agent/package_size"


class PackageSize:
    def __init__(self, arch, flavor, os_name, threshold):
        self.arch = arch
        self.flavor = flavor
        self.os = os_name
        self.size = 0
        self.ancestor_size = 0
        self.diff = 0
        self.mb_diff = 0
        self.threshold = threshold
        self.emoji = "✅"

    @property
    def name(self):
        return f"{self.flavor}-{self.arch}-{self.os}"

    def arch_name(self):
        if self.arch in ["x86_64", "amd64"]:
            return "amd"
        return "arm"

    def ko(self):
        return self.diff > self.threshold

    def path(self):
        if self.os == 'suse':
            dir = os.environ['OMNIBUS_PACKAGE_DIR_SUSE']
            return f'{dir}/{self.flavor}-7*{self.arch}.rpm'
        else:
            dir = os.environ['OMNIBUS_PACKAGE_DIR']
            separator = '_' if self.os == 'deb' else '-'
            return f'{dir}/{self.flavor}{separator}7*{self.arch}.{self.os}'

    def compare(self, size, ancestor_size):
        self.size = size
        self.ancestor_size = ancestor_size
        self.diff = self.size - self.ancestor_size
        self.mb_diff = float(f"{self.diff / pow(10, 6):.2f}")
        if self.ko():
            self.emoji = "❌"
        elif self.mb_diff > 0:
            self.emoji = "⚠️"

    @staticmethod
    def mb(value):
        return f"{value / 1e6:.2f}MB"

    def log(self):
        return f"{self.emoji} - {self.name} size {self.mb(self.size)}: {self.mb(self.diff)} diff[{self.diff}] with previous {self.mb(self.ancestor_size)} (max: {self.mb(self.threshold)})"

    def markdown(self):
        elements = (
            self.name,
            self.mb(self.diff),
            self.emoji,
            self.mb(self.size),
            self.mb(self.ancestor_size),
            self.mb(self.threshold),
        )
        return f'|{"|".join(map(str, elements))}|'


def find_package(glob_pattern):
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
            if key != "timestamp":
                packs.append(parent + [key, value])
    return packs


def retrieve_package_sizes(ctx, package_size_file: str, distant: bool = True):
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
        if distant:
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


def upload_package_sizes(ctx, package_sizes: dict, package_size_file: str, distant: bool = True):
    """
    Save the package_sizes dict to a file and upload it to the CI bucket
    """
    with open(package_size_file, "w") as f:
        json.dump(package_sizes, f)
    if distant:
        ctx.run(
            f"{AWS_S3_CP_CMD} {package_size_file} {PACKAGE_SIZE_S3_CI_BUCKET_URL}/{package_size_file}",
            hide="stdout",
        )


def get_ancestor(ctx, package_sizes, on_main):
    """
    Get the common ancestor between HEAD and the default branch
    Return the most recent commit if the ancestor is not found in the package_size file
    """
    ancestor = get_common_ancestor(ctx, "HEAD")
    if not on_main and ancestor not in package_sizes:
        return max(package_sizes, key=lambda x: package_sizes[x]['timestamp'])
    return ancestor


def display_message(ctx, ancestor, rows, reduction_rows, decision):
    is_open = '' if "Passed" in decision else ' open'
    size_wins = f"""<details open>
<summary> Size reduction summary </summary>

|package|diff|status|size|ancestor|threshold|
|--|--|--|--|--|--|
{reduction_rows}
</details>
"""
    message = f"""Comparison with [ancestor](https://github.com/DataDog/datadog-agent/commit/{ancestor}) `{ancestor}`
{size_wins if reduction_rows else ''}

<details{is_open}>
  <summary> Diff per package </summary>

|package|diff|status|size|ancestor|threshold|
|--|--|--|--|--|--|
{rows}
</details>

## Decision
{decision}

{"Currently this PR is blocked, you can reach out to #agent-delivery-help to get support/ask for an exception." if "❌" in decision else ""}
"""
    pr_commenter(ctx, title="Uncompressed package size comparison", body=message)
