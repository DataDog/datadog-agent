"""
Cluster Agent tasks
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
from .go import deps

# constants
BIN_PATH = os.path.join(".", "bin", "datadog-cluster-agent")
AGENT_TAG = "datadog/cluster_agent:master"
DEFAULT_BUILD_TAGS = [
    "kubeapiserver",
    "clusterchecks",
    "secrets",
]


@task
def build(ctx, rebuild=False, build_include=None, build_exclude=None,
          race=False, development=True, skip_assets=False):
    """
    Build Cluster Agent

     Example invokation:
        inv cluster-agent.build
    """
    build_include = DEFAULT_BUILD_TAGS if build_include is None else build_include.split(",")
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    build_tags = get_build_tags(build_include, build_exclude)

    # We rely on the go libs embedded in the debian stretch image to build dynamically
    ldflags, gcflags, env = get_build_flags(ctx, static=False, prefix='dca')

    cmd = "go build {race_opt} {build_type} -tags '{build_tags}' -o {bin_name} "
    cmd += "-gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/cluster-agent"
    args = {
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else "-i",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(BIN_PATH, bin_name("datadog-cluster-agent")),
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

    cmd = "go generate -tags '{build_tags}' {repo_path}/cmd/cluster-agent"
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
        os.path.join("./Dockerfiles/cluster-agent", "dist")
        ]
    for dist_folder in dist_folders:
        if os.path.exists(dist_folder):
            shutil.rmtree(dist_folder)
        copy_tree("./pkg/status/dist/", dist_folder)
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
    print(get_version(ctx, include_git=True, url_safe=url_safe, git_sha_length=git_sha_length, prefix='dca'))
