"""
Common utilities for building Cluster Agent variants
"""

import os
import shutil

from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags, get_version
from tasks.schema.generate import compress as schema_compress
from tasks.schema.template import CORE_SCHEMA_FILE, generate_template

# Maps cluster-agent binary suffix to (build_type, output file).
# Empty suffix -> mainline cluster-agent (dca); -cloudfoundry -> dcacf.
_CLUSTER_AGENT_RENDER_TARGETS = {
    "": ("dca", "./Dockerfiles/cluster-agent/datadog-cluster.yaml"),
    "-cloudfoundry": ("dcacf", "./cloudfoundry.yaml"),
}


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
    cover=False,
):
    """
    Build Cluster Agent
    """

    # We rely on the go libs embedded in the debian stretch image to build dynamically
    ldflags, gcflags, env = get_build_flags(ctx, static=False)

    schema_compress(ctx)

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
        check_deadcode=os.getenv("DEPLOY_AGENT") == "true",
        coverage=cover,
    )

    # Render the configuration file template. The cluster-agent and the
    # cloudfoundry variant only ship on linux, so we always target linux
    # (matches the legacy `go generate` behavior on the native build host).
    build_type, output = _CLUSTER_AGENT_RENDER_TARGETS[bin_suffix]
    generate_template(CORE_SCHEMA_FILE, output, build_type, "linux")

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
