"""
Cluster Agent tasks
"""

import glob
import os
import shutil

from invoke import task
from invoke.exceptions import Exit

from .build_tags import get_build_tags, get_default_build_tags
from .cluster_agent_helpers import build_common, clean_common, refresh_assets_common, version_common
from .go import deps
from .utils import load_release_versions

# constants
BIN_PATH = os.path.join(".", "bin", "datadog-cluster-agent")
AGENT_TAG = "datadog/cluster_agent:master"
POLICIES_REPO = "https://github.com/DataDog/security-agent-policies.git"


@task
def build(
    ctx,
    rebuild=False,
    build_include=None,
    build_exclude=None,
    race=False,
    development=True,
    skip_assets=False,
    policies_version=None,
    release_version="nightly-a7",
):
    """
    Build Cluster Agent

     Example invokation:
        inv cluster-agent.build
    """
    build_common(
        ctx,
        BIN_PATH,
        get_default_build_tags(build="cluster-agent"),
        "",
        rebuild,
        build_include,
        build_exclude,
        race,
        development,
        skip_assets,
    )

    if policies_version is None:
        print("Loading release versions for {}".format(release_version))
        env = load_release_versions(ctx, release_version)
        if "SECURITY_AGENT_POLICIES_VERSION" in env:
            policies_version = env["SECURITY_AGENT_POLICIES_VERSION"]
            print("Security Agent polices for {}: {}".format(release_version, policies_version))

    build_context = "Dockerfiles/cluster-agent"
    policies_path = "{}/security-agent-policies".format(build_context)
    ctx.run("rm -rf {}".format(policies_path))
    ctx.run("git clone {} {}".format(POLICIES_REPO, policies_path))
    if policies_version != "master":
        ctx.run("cd {} && git checkout {}".format(policies_path, policies_version))


@task
def refresh_assets(ctx, development=True):
    """
    Clean up and refresh cluster agent's assets and config files
    """
    refresh_assets_common(ctx, BIN_PATH, [os.path.join("./Dockerfiles/cluster-agent", "dist")], development)


@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    clean_common(ctx, "datadog-cluster-agent")


@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False, go_mod="mod"):
    """
    Run integration tests for cluster-agent
    """
    if install_deps:
        deps(ctx)

    # We need docker for the kubeapiserver integration tests
    tags = get_default_build_tags(build="cluster-agent") + ["docker"]

    test_args = {
        "go_mod": go_mod,
        "go_build_tags": " ".join(get_build_tags(tags, [])),
        "race_opt": "-race" if race else "",
        "exec_opts": "",
    }

    # since Go 1.13, the -exec flag of go test could add some parameters such as -test.timeout
    # to the call, we don't want them because while calling invoke below, invoke
    # thinks that the parameters are for it to interpret.
    # we're calling an intermediate script which only pass the binary name to the invoke task.
    if remote_docker:
        test_args["exec_opts"] = "-exec \"{}/test/integration/dockerize_tests.sh\"".format(os.getcwd())

    go_cmd = 'go test -mod={go_mod} {race_opt} -tags "{go_build_tags}" {exec_opts}'.format(**test_args)

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
    exec_path = "{}/datadog-cluster-agent.{}".format(build_context, arch)
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
