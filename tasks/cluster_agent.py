"""
Cluster Agent tasks
"""

import os
import glob
import shutil

from invoke import task
import invoke
from invoke.exceptions import Exit

from .build_tags import get_build_tags
from .utils import get_build_flags, bin_name
from .utils import REPO_PATH
from .go import deps

#constants
BIN_PATH = os.path.join(".", "bin", "datadog-cluster-agent")
AGENT_TAG = "datadog/cluster_agent:master"
DEFAULT_BUILD_TAGS = [
    "kubeapiserver",
]
@task
def build(ctx, rebuild=False, race=False, static=False, use_embedded_libs=False):
    """
    Build Cluster Agent

     Example invokation:
        inv cluster-agent.build
    """

    build_tags = get_build_tags(DEFAULT_BUILD_TAGS, [])

    ldflags, gcflags = get_build_flags(ctx, static=static, use_embedded_libs=use_embedded_libs)

    cmd = "go build {race_opt} {build_type} -tags '{build_tags}' -o {bin_name} "
    cmd += "-gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/cluster-agent/"
    args = {
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else "-i",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(BIN_PATH, bin_name("datadog-cluster-agent")),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args))

@task
def run(ctx, rebuild=False, race=False, skip_build=False, development=True):
    """
    Run the Cluster Agent's binary. Build the binary before executing, unless
    --skip-build was passed.
    """
    if not skip_build:
        print("Building the Cluster Agent...")
        build(ctx, rebuild=rebuild, race=race)

    target = os.path.join(BIN_PATH, bin_name("datadog-cluster-agent"))
    cfgPath = ""
    if development:
        cfgPath = "-c dev/dist/datadog-cluster.yaml"

    ctx.run("{0} start {1}".format(target, cfgPath))

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


@task(help={'skip-sign': "On macOS, use this option to build an unsigned package if you don't have Datadog's developer keys."})
def omnibus_build(ctx, log_level="info", base_dir=None, gem_path=None,
                  skip_deps=False, skip_sign=False):
    """
    Build the Cluster Agent packages with Omnibus Installer.
    """
    if not skip_deps:
        deps(ctx)

    # omnibus config overrides
    overrides = []

    # base dir (can be overridden through env vars, command line takes precedence)
    base_dir = base_dir or os.environ.get("OMNIBUS_BASE_DIR")
    if base_dir:
        overrides.append("base_dir:{}".format(base_dir))

    overrides_cmd = ""
    if overrides:
        overrides_cmd = "--override=" + " ".join(overrides)

    with ctx.cd("omnibus"):
        cmd = "bundle install"
        if gem_path:
            cmd += " --path {}".format(gem_path)
        ctx.run(cmd)
        omnibus = "bundle exec omnibus.bat" if invoke.platform.WINDOWS else "bundle exec omnibus"
        cmd = "{omnibus} build {project_name} --log-level={log_level} {overrides}"
        args = {
            "omnibus": omnibus,
            "project_name": "datadog-cluster-agent",
            "log_level": log_level,
            "overrides": overrides_cmd
        }
        env = {}
        if skip_sign:
            env['SKIP_SIGN_MAC'] = 'true'
        ctx.run(cmd.format(**args), env=env)


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
    ]

    for prefix in prefixes:
        ctx.run("{} {}".format(go_cmd, prefix))


@task
def image_build(ctx, base_dir="omnibus"):
    """
    Build the docker image
    """
    base_dir = base_dir or os.environ.get("OMNIBUS_BASE_DIR")
    pkg_dir = os.path.join(base_dir, 'pkg')
    list_of_files = glob.glob(os.path.join(pkg_dir, 'datadog-cluster-agent*_amd64.deb'))
    # get the last debian package built
    if not list_of_files:
        print("No debian package build found in {}".format(pkg_dir))
        print("See agent.omnibus-build")
        raise Exit(1)
    latest_file = max(list_of_files, key=os.path.getctime)
    shutil.copy2(latest_file, "Dockerfiles/cluster-agent/")
    ctx.run("docker build -t {} Dockerfiles/cluster-agent".format(AGENT_TAG))
    ctx.run("rm Dockerfiles/cluster-agent/datadog-cluster-agent*_amd64.deb")
