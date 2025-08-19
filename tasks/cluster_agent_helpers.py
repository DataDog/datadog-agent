"""
Common utilities for building Cluster Agent variants
"""

import os
import shutil

from tasks.build_tags import filter_incompatible_tags, get_build_tags
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags, get_version


def build_common(
    ctx,
    bin_path,
    build_tags,
    bin_suffix,
    rebuild,
    build_include,
    build_exclude,
    race,
    development,
    skip_assets,
    go_mod="readonly",
    major_version="7",
    cover=False,
):
    """
    Build Cluster Agent
    """

    build_include = build_tags if build_include is None else filter_incompatible_tags(build_include.split(","))
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    build_tags = get_build_tags(build_include, build_exclude)

    # We rely on the go libs embedded in the debian stretch image to build dynamically
    ldflags, gcflags, env = get_build_flags(ctx, static=False, major_version=major_version)

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/cluster-agent{bin_suffix}",
        mod=go_mod,
        race=race,
        rebuild=rebuild,
        gcflags=gcflags,
        ldflags=ldflags,
        build_tags=build_tags,
        bin_path=os.path.join(bin_path, bin_name(f"datadog-cluster-agent{bin_suffix}")),
        env=env,
        coverage=cover,
    )

    # Render the configuration file template
    #
    # We need to remove cross compiling bits if any because go generate must
    # build and execute in the native platform
    env.update({"GOOS": "", "GOARCH": ""})

    cmd = "go generate -mod={go_mod} -tags '{build_tags}' {repo_path}/cmd/cluster-agent{suffix}"
    ctx.run(cmd.format(go_mod=go_mod, build_tags=" ".join(build_tags), repo_path=REPO_PATH, suffix=bin_suffix), env=env)

    if not skip_assets:
        refresh_assets_common(
            ctx, bin_path, [os.path.join("./Dockerfiles/cluster-agent", "dist")], development=development
        )


def refresh_assets_common(_, bin_path, additional_dist_folders, development):
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
            shutil.copytree("./dev/dist/", dist_folder, dirs_exist_ok=True)


def clean_common(ctx, rmdir):
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/agent folder
    print("Remove agent binary folder")
    ctx.run(f"rm -rf ./bin/{rmdir}")


def version_common(ctx, url_safe, git_sha_length):
    """
    Get the agent version.
    """
    print(get_version(ctx, include_git=True, url_safe=url_safe, git_sha_length=git_sha_length))
