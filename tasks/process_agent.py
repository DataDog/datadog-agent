import os
import shutil
import sys
import tempfile

from invoke import task
from invoke.exceptions import Exit

from .build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from .flavor import AgentFlavor
from .utils import REPO_PATH, bin_name, get_build_flags, get_version_numeric_only

BIN_DIR = os.path.join(".", "bin", "process-agent")
BIN_PATH = os.path.join(BIN_DIR, bin_name("process-agent"))


@task
def build(
    ctx,
    race=False,
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
    incremental_build=False,
    major_version='7',
    python_runtimes='3',
    arch="x64",
    go_mod="mod",
):
    """
    Build the process agent
    """
    flavor = AgentFlavor[flavor]
    ldflags, gcflags, env = get_build_flags(ctx, major_version=major_version, python_runtimes=python_runtimes)

    # generate windows resources
    if sys.platform == 'win32':
        windres_target = "pe-x86-64"
        if arch == "x86":
            env["GOARCH"] = "386"
            windres_target = "pe-i386"

        ver = get_version_numeric_only(ctx, major_version=major_version)
        maj_ver, min_ver, patch_ver = ver.split(".")
        resdir = os.path.join(".", "cmd", "process-agent", "windows_resources")

        ctx.run(f"windmc --target {windres_target} -r {resdir} {resdir}/process-agent-msg.mc")

        ctx.run(
            f"windres --define MAJ_VER={maj_ver} --define MIN_VER={min_ver} --define PATCH_VER={patch_ver} -i cmd/process-agent/windows_resources/process-agent.rc --target {windres_target} -O coff -o cmd/process-agent/rsrc.syso"
        )

    goenv = {}
    # extend PATH from gimme with the one from get_build_flags
    if "PATH" in os.environ and "PATH" in goenv:
        goenv["PATH"] += ":" + os.environ["PATH"]
    env.update(goenv)

    build_include = (
        get_default_build_tags(build="process-agent", arch=arch, flavor=flavor)
        if build_include is None
        else filter_incompatible_tags(build_include.split(","), arch=arch)
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    build_tags = get_build_tags(build_include, build_exclude)

    # TODO static option
    cmd = 'go build -mod={go_mod} {race_opt} {build_type} -tags "{go_build_tags}" '
    cmd += '-o {agent_bin} -gcflags="{gcflags}" -ldflags="{ldflags}" {REPO_PATH}/cmd/process-agent'

    args = {
        "go_mod": go_mod,
        "race_opt": "-race" if race else "",
        "build_type": "" if incremental_build else "-a",
        "go_build_tags": " ".join(build_tags),
        "agent_bin": BIN_PATH,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }

    ctx.run(cmd.format(**args), env=env)


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

        ctx.run(f"cp pkg/ebpf/bytecode/build/*.o {docker_context}")
        ctx.run(f"cp pkg/ebpf/bytecode/build/runtime/*.c {docker_context}")
        ctx.run(f"chmod 0444 {docker_context}/*.o {docker_context}/*.c")
        ctx.run(f"cp /opt/datadog-agent/embedded/bin/clang-bpf {docker_context}")
        ctx.run(f"cp /opt/datadog-agent/embedded/bin/llc-bpf {docker_context}")

        with ctx.cd(docker_context):
            # --pull in the build will force docker to grab the latest base image
            ctx.run(
                f"docker build --pull --tag {image} --build-arg AGENT_BASE={base_image} --build-arg CORE_AGENT_DEST={core_agent_dest} ."
            )

    if push:
        ctx.run(f"docker push {image}")


@task
def go_generate(ctx):
    """
    Run the go generate directives inside the /pkg/process directory

    """
    with ctx.cd("./pkg/process/events/model"):
        ctx.run("go generate ./...")


@task
def gen_mocks(ctx):
    """
    Generate mocks
    """

    interfaces = {
        "./pkg/process/checks": ["Check", "CheckWithRealTime"],
        "./pkg/process/net": ["SysProbeUtil"],
        "./pkg/process/procutil": ["Probe"],
    }

    for path, names in interfaces.items():
        interface_regex = "|".join(f"^{i}\\$" for i in names)

        with ctx.cd(path):
            ctx.run(f"mockery --case snake --name=\"{interface_regex}\"")
