"""
Common utilities for building Cluster Agent variants
"""

import os
import shutil
from distutils.dir_util import copy_tree

from .build_tags import get_build_tags
from .utils import get_build_flags, bin_name, get_version
from .utils import REPO_PATH
from .go import generate


def build_common(ctx, command, bin_path, build_tags, bin_suffix, rebuild, build_include,
                 build_exclude, race, development, skip_assets, go_mod="vendor"):
    """
    Build Cluster Agent
    """

    build_include = build_tags if build_include is None else build_include.split(",")
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    build_tags = get_build_tags(build_include, build_exclude)

    # We rely on the go libs embedded in the debian stretch image to build dynamically
    ldflags, gcflags, env = get_build_flags(ctx, static=False, prefix='dca')

    # Generating go source from templates by running go generate on ./pkg/status
    generate(ctx)

    cmd = "go build -mod={go_mod} {race_opt} {build_type} -tags '{build_tags}' -o {bin_name} "
    cmd += "-gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/cluster-agent{suffix}"
    args = {
        "go_mod": go_mod,
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else "",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(
            bin_path, bin_name("datadog-cluster-agent{suffix}".format(suffix=bin_suffix))),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
        "suffix": bin_suffix,
    }

    ctx.run(cmd.format(**args), env=env)
    # Render the configuration file template
    #
    # We need to remove cross compiling bits if any because go generate must
    # build and execute in the native platform
    env.update({
        "GOOS": "",
        "GOARCH": "",
    })

    cmd = "go generate -mod={go_mod} -tags '{build_tags}' {repo_path}/cmd/cluster-agent{suffix}"
    ctx.run(cmd.format(go_mod=go_mod, build_tags=" ".join(build_tags), repo_path=REPO_PATH, suffix=bin_suffix), env=env)

    if not skip_assets:
        refresh_assets_common(
            ctx,
            bin_path,
            [os.path.join("./Dockerfiles/cluster-agent", "dist")],
            development=development
        )


def refresh_assets_common(ctx, bin_path, additional_dist_folders, development):
    """
    Clean up and refresh cluster agent's assets and config files
    """
    # ensure BIN_PATH exists
    if not os.path.exists(bin_path):
        os.mkdir(bin_path)

    dist_folders = [
        os.path.join(bin_path, "dist"),
        ]
    dist_folders.extend(additional_dist_folders)
    for dist_folder in dist_folders:
        if os.path.exists(dist_folder):
            shutil.rmtree(dist_folder)
        if development:
            copy_tree("./dev/dist/", dist_folder)


def clean_common(ctx, rmdir):
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/agent folder
    print("Remove agent binary folder")
    ctx.run("rm -rf ./bin/{rmdir}".format(rmdir=rmdir))


def version_common(ctx, url_safe, git_sha_length):
    """
    Get the agent version.
    """
    print(get_version(ctx, include_git=True, url_safe=url_safe, git_sha_length=git_sha_length, prefix='dca'))
