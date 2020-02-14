"""
Cluster Agent for Cloud Foundry tasks
"""

import os
import glob
import shutil
from distutils.dir_util import copy_tree

from invoke import task
from invoke.exceptions import Exit

from .build_tags import get_build_tags
from .utils import get_build_flags, bin_name, get_version
from .utils import REPO_PATH
from .go import deps, generate

# constants
BIN_PATH = os.path.join(".", "bin", "datadog-cluster-agent-cloudfoundry")
DEFAULT_BUILD_TAGS = [
    "clusterchecks",
    "secrets",
]


@task
def build(ctx, rebuild=False, build_include=None, build_exclude=None,
          race=False, development=True, skip_assets=False):
    """
    Build Cluster Agent for Cloud Foundry

     Example invokation:
        inv cluster-agent-cloudfoundry.build
    """
    build_include = DEFAULT_BUILD_TAGS if build_include is None else build_include.split(",")
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    build_tags = get_build_tags(build_include, build_exclude)

    # We rely on the go libs embedded in the debian stretch image to build dynamically
    ldflags, gcflags, env = get_build_flags(ctx, static=False, prefix='dca')

    # Generating go source from templates by running go generate on ./pkg/status
    generate(ctx)

    cmd = "go build {race_opt} {build_type} -tags '{build_tags}' -o {bin_name} "
    cmd += "-gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/cluster-agent-cloudfoundry"
    args = {
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else "-i",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(BIN_PATH, bin_name("datadog-cluster-agent-cloudfoundry")),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
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

    cmd = "go generate -tags '{build_tags}' {repo_path}/cmd/cluster-agent-cloudfoundry"
    ctx.run(cmd.format(build_tags=" ".join(build_tags), repo_path=REPO_PATH), env=env)

    if not skip_assets:
        refresh_assets(ctx, development=development)

@task
def refresh_assets(ctx, development=True):
    """
    Clean up and refresh cluster agent's assets and config files
    """
    # ensure BIN_PATH exists
    if not os.path.exists(BIN_PATH):
        os.mkdir(BIN_PATH)

    dist_folders = [
        os.path.join(BIN_PATH, "dist"),
        ]
    for dist_folder in dist_folders:
        if os.path.exists(dist_folder):
            shutil.rmtree(dist_folder)
        if development:
            copy_tree("./dev/dist/", dist_folder)

@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/agent folder
    print("Remove agent binary folder")
    ctx.run("rm -rf ./bin/datadog-cluster-agent")


@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False):
    """
    Run integration tests for cluster-agent-cloudfoundry
    """
    pass  # TODO


@task
def version(ctx, url_safe=False, git_sha_length=7):
    """
    Get the agent version.
    url_safe: get the version that is able to be addressed as a url
    git_sha_length: different versions of git have a different short sha length,
                    use this to explicitly set the version
                    (the windows builder and the default ubuntu version have such an incompatibility)
    """
    print(get_version(ctx, include_git=True, url_safe=url_safe, git_sha_length=git_sha_length, prefix='dca'))
