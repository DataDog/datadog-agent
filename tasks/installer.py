"""
installer namespaced tasks
"""

import os

from invoke import task

from tasks.build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags

BIN_PATH = os.path.join(".", "bin", "installer")
MAJOR_VERSION = '7'


@task
def build(
    ctx,
    rebuild=False,
    race=False,
    install_path=None,
    build_include=None,
    build_exclude=None,
    arch="x64",
    go_mod="mod",
):
    """
    Build the updater.
    """

    ldflags, gcflags, env = get_build_flags(ctx, major_version=MAJOR_VERSION, install_path=install_path)

    build_include = (
        get_default_build_tags(
            build="updater",
        )  # TODO/FIXME: Arch not passed to preserve build tags. Should this be fixed?
        if build_include is None
        else filter_incompatible_tags(build_include.split(","), arch=arch)
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    build_tags = get_build_tags(build_include, build_exclude)

    race_opt = "-race" if race else ""
    build_type = "-a" if rebuild else ""
    go_build_tags = " ".join(build_tags)
    updater_bin = os.path.join(BIN_PATH, bin_name("installer"))
    cmd = f"go build -mod={go_mod} {race_opt} {build_type} -tags \"{go_build_tags}\" "
    cmd += f"-o {updater_bin} -gcflags=\"{gcflags}\" -ldflags=\"{ldflags} -w -s\" {REPO_PATH}/cmd/installer"

    ctx.run(cmd, env=env)
