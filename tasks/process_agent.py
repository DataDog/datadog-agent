import os
import shutil
import sys
import tempfile

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import (
    compute_build_tags_for_flavor,
)
from tasks.flavor import AgentFlavor
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags
from tasks.system_probe import copy_ebpf_and_related_files
from tasks.windows_resources import build_messagetable, build_rc, versioninfo_vars

BIN_DIR = os.path.join(".", "bin", "process-agent")
BIN_PATH = os.path.join(BIN_DIR, bin_name("process-agent"))


@task
def build(
    ctx,
    race=False,
    build_include=None,
    build_exclude=None,
    install_path=None,
    flavor=AgentFlavor.base.name,
    rebuild=False,
    go_mod="readonly",
):
    """
    Build the process agent
    """

    flavor = AgentFlavor[flavor]

    ldflags, gcflags, env = get_build_flags(
        ctx,
        install_path=install_path,
    )

    # generate windows resources
    if sys.platform == 'win32':
        build_messagetable(ctx)
        vars = versioninfo_vars(ctx)
        build_rc(
            ctx,
            "cmd/process-agent/windows_resources/process-agent.rc",
            vars=vars,
            out="cmd/process-agent/rsrc.syso",
        )

    goenv = {}
    # extend PATH from gimme with the one from get_build_flags
    if "PATH" in os.environ and "PATH" in goenv:
        goenv["PATH"] += ":" + os.environ["PATH"]
    env.update(goenv)

    build_tags = compute_build_tags_for_flavor(
        build="process-agent", flavor=flavor, build_include=build_include, build_exclude=build_exclude
    )

    if os.path.exists(BIN_PATH):
        os.remove(BIN_PATH)

    # TODO static option
    go_build(
        ctx,
        f"{REPO_PATH}/cmd/process-agent",
        mod=go_mod,
        race=race,
        rebuild=rebuild,
        gcflags=gcflags,
        ldflags=ldflags,
        build_tags=build_tags,
        bin_path=BIN_PATH,
        env=env,
        check_deadcode=os.getenv("DEPLOY_AGENT") == "true",
        coverage=os.getenv("E2E_COVERAGE_PIPELINE") == "true",
    )


class TempDir:
    def __enter__(self):
        self.fname = tempfile.mkdtemp()
        print(f"created tempdir: {self.fname}")
        return self.fname

    # The _ in front of the unused arguments are needed to pass lint check
    def __exit__(self, _exception_type, _exception_value, _exception_traceback):
        print(f"deleting tempdir: {self.fname}")
        shutil.rmtree(self.fname)


@task
def build_dev_image(ctx, image=None, push=False, base_image="datadog/agent:latest", include_agent_binary=False):
    """
    Build a dev image of the process-agent based off an existing datadog-agent image

    image: the image name used to tag the image
    push: if true, run a docker push on the image
    base_image: base the docker image off this already build image (default: datadog/agent:latest)
    include_agent_binary: if true, use the agent binary in bin/agent/agent as opposite to the base image's binary
    """
    if image is None:
        raise Exit(message="image was not specified")

    with TempDir() as docker_context:
        ctx.run(f"cp tools/ebpf/Dockerfiles/Dockerfile-process-agent-dev {docker_context + '/Dockerfile'}")

        ctx.run(f"cp bin/process-agent/process-agent {docker_context + '/process-agent'}")
        ctx.run(f"cp bin/system-probe/system-probe {docker_context + '/system-probe'}")
        if include_agent_binary:
            ctx.run(f"cp bin/agent/agent {docker_context + '/agent'}")
            core_agent_dest = "/opt/datadog-agent/bin/agent/agent"
        else:
            # this is necessary so that the docker build doesn't fail while attempting to copy the agent binary
            ctx.run(f"touch {docker_context}/agent")
            core_agent_dest = "/dev/null"

        copy_ebpf_and_related_files(ctx, docker_context)

        with ctx.cd(docker_context):
            # --pull in the build will force docker to grab the latest base image
            ctx.run(
                f"docker build --pull --tag {image} --build-arg AGENT_BASE={base_image} --build-arg CORE_AGENT_DEST={core_agent_dest} ."
            )

    if push:
        ctx.run(f"docker push {image}")


@task
def gen_mocks(ctx):
    """
    Generate mocks
    """
    ctx.run("mockery")
