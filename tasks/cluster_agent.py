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

from tasks.build_tags import get_build_tags, get_default_build_tags
from tasks.cluster_agent_helpers import build_common, clean_common, refresh_assets_common, version_common
from tasks.go import deps
from tasks.libs.common.utils import load_release_versions

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
        print(f"Loading release versions for {release_version}")
        env = load_release_versions(ctx, release_version)
        if "SECURITY_AGENT_POLICIES_VERSION" in env:
            policies_version = env["SECURITY_AGENT_POLICIES_VERSION"]
            print(f"Security Agent polices for {release_version}: {policies_version}")

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
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False, go_mod="mod"):
    """
    Run integration tests for cluster-agent
    """
    if sys.platform == 'win32':
        raise Exit(message='cluster-agent integration tests are not supported on Windows', code=0)

    if install_deps:
        deps(ctx)

    # We need docker for the kubeapiserver integration tests
    tags = get_default_build_tags(build="cluster-agent") + ["docker", "test"]

    go_build_tags = " ".join(get_build_tags(tags, []))
    race_opt = "-race" if race else ""
    exec_opts = ""

    # since Go 1.13, the -exec flag of go test could add some parameters such as -test.timeout
    # to the call, we don't want them because while calling invoke below, invoke
    # thinks that the parameters are for it to interpret.
    # we're calling an intermediate script which only pass the binary name to the invoke task.
    if remote_docker:
        exec_opts = f"-exec \"{os.getcwd()}/test/integration/dockerize_tests.sh\""

    go_cmd = f'go test -mod={go_mod} {race_opt} -tags "{go_build_tags}" {exec_opts}'

    prefixes = [
        "./test/integration/util/kube_apiserver",
        "./test/integration/util/leaderelection",
    ]

    for prefix in prefixes:
        ctx.run(f"{go_cmd} {prefix}")


@task
def image_build(ctx, arch=None, tag=AGENT_TAG, push=False):
    """
    Build the docker image
    """
    if arch is None:
        arch = CONTAINER_PLATFORM_MAPPING.get(platform.machine().lower())

    if arch is None:
        print("Unable to determine architecture to build, please set `arch` parameter")
        raise Exit(code=1)

    dca_binary = glob.glob(os.path.join(BIN_PATH, "datadog-cluster-agent"))
    # get the last debian package built
    if not dca_binary:
        print(f"No bin found in {BIN_PATH}")
        print("See cluster-agent.build")
        raise Exit(code=1)
    latest_file = max(dca_binary, key=os.path.getctime)
    ctx.run(f"chmod +x {latest_file}")

    build_context = "Dockerfiles/cluster-agent"
    exec_path = f"{build_context}/datadog-cluster-agent.{arch}"
    dockerfile_path = f"{build_context}/Dockerfile"

    shutil.copy2(latest_file, exec_path)
    shutil.copytree("Dockerfiles/agent/nosys-seccomp", f"{build_context}/nosys-seccomp", dirs_exist_ok=True)
    ctx.run(f"docker build -t {tag} --platform linux/{arch} {build_context} -f {dockerfile_path}")
    ctx.run(f"rm {exec_path}")

    if push:
        ctx.run(f"docker push {tag}")


@task
def hacky_dev_image_build(ctx, base_image=None, target_image="cluster-agent", push=False, signed_pull=False):
    os.environ["DELVE"] = "1"
    build(ctx)

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
        ctx.run(f'docker build -t {target_image} -f {dockerfile.name} .', env=pull_env)

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


@task
def update_generated_code(ctx):
    """
    Re-generate 'pkg/clusteragent/autoscaling/custommetrics/api/generated/openapi/zz_generated.openapi.go'.
    """
    ctx.run("go install -mod=readonly k8s.io/kube-openapi/cmd/openapi-gen")
    ctx.run(
        "$GOPATH/bin/openapi-gen \
--logtostderr \
-i k8s.io/metrics/pkg/apis/custom_metrics,k8s.io/metrics/pkg/apis/custom_metrics/v1beta1,k8s.io/metrics/pkg/apis/custom_metrics/v1beta2,k8s.io/metrics/pkg/apis/external_metrics,k8s.io/metrics/pkg/apis/external_metrics/v1beta1,k8s.io/metrics/pkg/apis/metrics,k8s.io/metrics/pkg/apis/metrics/v1beta1,k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/api/resource,k8s.io/apimachinery/pkg/version,k8s.io/api/core/v1 \
-h ./tools/boilerplate.go.txt \
-p ./pkg/clusteragent/autoscaling/custommetrics/api/generated/openapi \
-O zz_generated.openapi \
-o ./ \
-r /dev/null"
    )
