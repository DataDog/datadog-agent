"""
Cluster Agent tasks
"""

import os
import glob
import shutil

from invoke import task
from invoke.exceptions import Exit

from .build_tags import get_build_tags
from .cluster_agent_helpers import build_common, clean_common, refresh_assets_common, version_common
from .go import deps

# constants
BIN_PATH = os.path.join(".", "bin", "datadog-cluster-agent")
AGENT_TAG = "datadog/cluster_agent:master"
DEFAULT_BUILD_TAGS = [
    "kubeapiserver",
    "clusterchecks",
    "secrets",
    "orchestrator",
]


@task
def build(ctx, rebuild=False, build_include=None, build_exclude=None,
          race=False, development=True, skip_assets=False):
    """
    Build Cluster Agent

     Example invokation:
        inv cluster-agent.build
    """
    build_common(ctx, "cluster-agent.build", BIN_PATH, DEFAULT_BUILD_TAGS, "", rebuild, build_include,
                 build_exclude, race, development, skip_assets)


@task
def refresh_assets(ctx, development=True):
    """
    Clean up and refresh cluster agent's assets and config files
    """
    refresh_assets_common(
        ctx,
        BIN_PATH,
        [os.path.join("./Dockerfiles/cluster-agent", "dist")],
        development
    )


@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    clean_common(ctx, "datadog-cluster-agent")


@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False):
    """
    Run integration tests for cluster-agent
    """
    if install_deps:
        deps(ctx)

    # We need docker for the kubeapiserver integration tests
    tags = DEFAULT_BUILD_TAGS + ["docker"]

    test_args = {
        "go_build_tags": " ".join(get_build_tags(tags, [])),
        "race_opt": "-race" if race else "",
        "exec_opts": "",
    }

    if remote_docker:
        test_args["exec_opts"] = "-exec \"inv docker.dockerize-test\""

    go_cmd = 'go test {race_opt} -tags "{go_build_tags}" {exec_opts}'.format(**test_args)

    prefixes = [
        "./test/integration/util/kube_apiserver",
        "./test/integration/util/leaderelection",
    ]

    for prefix in prefixes:
        ctx.run("{} {}".format(go_cmd, prefix))


@task
def image_build(ctx, arch='amd64', tag=AGENT_TAG, push=False):
    """
    Build the docker image
    """

    dca_binary = glob.glob(os.path.join(BIN_PATH, "datadog-cluster-agent"))
    # get the last debian package built
    if not dca_binary:
        print("No bin found in {}".format(BIN_PATH))
        print("See cluster-agent.build")
        raise Exit(code=1)
    latest_file = max(dca_binary, key=os.path.getctime)
    ctx.run("chmod +x {}".format(latest_file))

    build_context = "Dockerfiles/cluster-agent"
    exec_path = "{}/datadog-cluster-agent.{}".format(build_context,arch)
    dockerfile_path = "{}/{}/Dockerfile".format(build_context, arch)

    shutil.copy2(latest_file, exec_path)
    ctx.run("docker build -t {} {} -f {}".format(tag, build_context, dockerfile_path))
    ctx.run("rm {}".format(exec_path))

    if push:
        ctx.run("docker push {}".format(tag))



@task
def version(ctx, url_safe=False, git_sha_length=7):
    """
    Get the agent version.
    url_safe: get the version that is able to be addressed as a url
    git_sha_length: different versions of git have a different short sha length,
                    use this to explicitly set the version
                    (the windows builder and the default ubuntu version have such an incompatibility)
    """
    version_common(ctx, url_safe, git_sha_length)
