"""
Cluster Agent tasks
"""

import glob
import os
import platform
import shutil
import sys
import tempfile

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import get_default_build_tags
from tasks.cluster_agent_helpers import build_common, clean_common, refresh_assets_common, version_common
from tasks.cws_instrumentation import BIN_PATH as CWS_INSTRUMENTATION_BIN_PATH
from tasks.gointegrationtest import CLUSTER_AGENT_IT_CONF, containerized_integration_tests
from tasks.libs.releasing.version import load_dependencies

# constants
BIN_PATH = os.path.join(".", "bin", "datadog-cluster-agent")
AGENT_TAG = "datadog/cluster_agent:master"
POLICIES_REPO = "https://github.com/DataDog/security-agent-policies.git"
CONTAINER_PLATFORM_MAPPING = {"aarch64": "arm64", "amd64": "amd64", "x86_64": "amd64"}


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
    major_version='7',
):
    """
    Build Cluster Agent

     Example invokation:
        dda inv cluster-agent.build
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
        major_version=major_version,
        cover=os.getenv("E2E_COVERAGE_PIPELINE") == "true",
    )

    if policies_version is None:
        print("Loading dependencies from release.json")
        env = load_dependencies(ctx)
        if "SECURITY_AGENT_POLICIES_VERSION" in env:
            policies_version = env["SECURITY_AGENT_POLICIES_VERSION"]
            print(f"Security Agent polices: {policies_version}")

    build_context = "Dockerfiles/cluster-agent"
    policies_path = f"{build_context}/security-agent-policies"
    ctx.run(f"rm -rf {policies_path}")
    ctx.run(f"git clone {POLICIES_REPO} {policies_path}")
    if policies_version != "master":
        ctx.run(f"cd {policies_path} && git checkout {policies_version}")


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
def integration_tests(ctx, race=False, remote_docker=False, go_mod="readonly", timeout=""):
    """
    Run integration tests for cluster-agent
    """
    containerized_integration_tests(
        ctx,
        CLUSTER_AGENT_IT_CONF,
        race=race,
        remote_docker=remote_docker,
        go_mod=go_mod,
        timeout=timeout,
    )


@task
def image_build(ctx, arch=None, tag=AGENT_TAG, push=False):
    """
    Build the docker image
    """
    if arch is None:
        arch = CONTAINER_PLATFORM_MAPPING.get(platform.machine().lower())

    if arch is None:
        print("Unable to determine architecture to build, please set `arch`", file=sys.stderr)
        raise Exit(code=1)

    dca_binary = glob.glob(os.path.join(BIN_PATH, "datadog-cluster-agent"))
    # get the last debian package built
    if not dca_binary:
        print(f"No bin found in {BIN_PATH}")
        print("See cluster-agent.build")
        raise Exit(code=1)
    latest_file = max(dca_binary, key=os.path.getctime)
    ctx.run(f"chmod +x {latest_file}")

    # add CWS instrumentation
    cws_instrumentation_binary = glob.glob(CWS_INSTRUMENTATION_BIN_PATH)
    if not cws_instrumentation_binary:
        print(f"No bin found in {CWS_INSTRUMENTATION_BIN_PATH}")
        print("You need to run cws-instrumentation.build first")
        raise Exit(code=1)
    latest_cws_instrumentation_file = max(cws_instrumentation_binary, key=os.path.getctime)
    ctx.run(f"chmod +x {latest_cws_instrumentation_file}")

    build_context = "Dockerfiles/cluster-agent"
    exec_path = f"{build_context}/datadog-cluster-agent"
    cws_instrumentation_base = f"{build_context}/cws-instrumentation"
    cws_instrumentation_exec_path = f"{cws_instrumentation_base}/cws-instrumentation.{arch}"

    dockerfile_path = f"{build_context}/Dockerfile"

    try:
        os.mkdir(cws_instrumentation_base)
    except FileExistsError:
        # Directory already exists
        pass
    except Exception as e:
        # Handle other OS-related errors
        print(f"Error creating directory: {e}")

    shutil.copy2(latest_file, exec_path)
    shutil.copy2(latest_cws_instrumentation_file, cws_instrumentation_exec_path)
    shutil.copytree("Dockerfiles/agent/nosys-seccomp", f"{build_context}/nosys-seccomp", dirs_exist_ok=True)
    ctx.run(
        f"docker build -t {tag} --platform linux/{arch} {build_context} -f {dockerfile_path} --build-context artifacts={build_context}"
    )
    ctx.run(f"rm {exec_path}")
    ctx.run(f"rm -rf {cws_instrumentation_base}")

    if push:
        ctx.run(f"docker push {tag}")


@task
def hacky_dev_image_build(
    ctx,
    base_image=None,
    target_image="cluster-agent",
    push=False,
    race=False,
    signed_pull=False,
    arch=None,
):
    os.environ["DELVE"] = "1"
    build(ctx, race=race)

    if arch is None:
        arch = CONTAINER_PLATFORM_MAPPING.get(platform.machine().lower())

    if arch is None:
        print("Unable to determine architecture to build, please set `arch`", file=sys.stderr)
        raise Exit(code=1)

    if base_image is None:
        import requests
        import semver

        # Try to guess what is the latest release of the cluster-agent
        latest_release = semver.VersionInfo(0)
        tags = requests.get("https://gcr.io/v2/datadoghq/cluster-agent/tags/list")
        for tag in tags.json()['tags']:
            if not semver.VersionInfo.isvalid(tag):
                continue
            ver = semver.VersionInfo.parse(tag)
            if ver.prerelease or ver.build:
                continue
            if ver > latest_release:
                latest_release = ver
        base_image = f"gcr.io/datadoghq/cluster-agent:{latest_release}"

    with tempfile.NamedTemporaryFile(mode='w') as dockerfile:
        dockerfile.write(
            f'''FROM ubuntu:latest AS src

COPY . /usr/src/datadog-agent

RUN find /usr/src/datadog-agent -type f \\! -name \\*.go -print0 | xargs -0 rm
RUN find /usr/src/datadog-agent -type d -empty -print0 | xargs -0 rmdir

FROM golang:latest AS dlv

RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM {base_image}

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get install -y bash-completion less vim tshark && \
    apt-get clean

ENV DELVE_PAGER=less

COPY --from=dlv /go/bin/dlv /usr/local/bin/dlv
COPY --from=src /usr/src/datadog-agent {os.getcwd()}
COPY bin/datadog-cluster-agent/datadog-cluster-agent /opt/datadog-agent/bin/datadog-cluster-agent
RUN agent                 completion bash > /usr/share/bash-completion/completions/agent
RUN datadog-cluster-agent completion bash > /usr/share/bash-completion/completions/datadog-cluster-agent

ENV DD_SSLKEYLOGFILE=/tmp/sslkeylog.txt
'''
        )
        dockerfile.flush()
        pull_env = {}
        if signed_pull:
            pull_env['DOCKER_CONTENT_TRUST'] = '1'
        ctx.run(f'docker build --platform linux/{arch} -t {target_image} -f {dockerfile.name} .', env=pull_env)

        if push:
            ctx.run(f'docker push {target_image}')


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
