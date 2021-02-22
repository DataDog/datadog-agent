import datetime
import os
import shutil
import sys
import tempfile

from invoke import task
from invoke.exceptions import Exit

from .build_tags import get_default_build_tags
from .utils import (
    REPO_PATH,
    bin_name,
    get_build_flags,
    get_git_branch_name,
    get_git_commit,
    get_go_version,
    get_version,
    get_version_numeric_only,
)

BIN_DIR = os.path.join(".", "bin", "process-agent")
BIN_PATH = os.path.join(BIN_DIR, bin_name("process-agent", android=False))
GIMME_ENV_VARS = ['GOROOT', 'PATH']


@task
def build(
    ctx,
    race=False,
    go_version=None,
    incremental_build=False,
    major_version='7',
    python_runtimes='3',
    arch="x64",
    go_mod="vendor",
):
    """
    Build the process agent
    """
    ldflags, gcflags, env = get_build_flags(
        ctx, arch=arch, major_version=major_version, python_runtimes=python_runtimes
    )

    # generate windows resources
    if sys.platform == 'win32':
        windres_target = "pe-x86-64"
        if arch == "x86":
            env["GOARCH"] = "386"
            windres_target = "pe-i386"

        ver = get_version_numeric_only(ctx, env, major_version=major_version)
        maj_ver, min_ver, patch_ver = ver.split(".")
        resdir = os.path.join(".", "cmd", "process-agent", "windows_resources")

        ctx.run(
            "windmc --target {target_arch} -r {resdir} {resdir}/process-agent-msg.mc".format(
                resdir=resdir, target_arch=windres_target
            )
        )

        ctx.run(
            "windres --define MAJ_VER={maj_ver} --define MIN_VER={min_ver} --define PATCH_VER={patch_ver} -i cmd/process-agent/windows_resources/process-agent.rc --target {target_arch} -O coff -o cmd/process-agent/rsrc.syso".format(
                maj_ver=maj_ver, min_ver=min_ver, patch_ver=patch_ver, target_arch=windres_target
            )
        )

    # TODO use pkg/version for this
    main = "main."
    ld_vars = {
        "Version": get_version(ctx, major_version=major_version),
        "GoVersion": get_go_version(),
        "GitBranch": get_git_branch_name(),
        "GitCommit": get_git_commit(),
        "BuildDate": datetime.datetime.now().strftime("%Y-%m-%dT%H:%M:%S"),
    }

    goenv = {}
    if go_version:
        lines = ctx.run("gimme {version}".format(version=go_version)).stdout.split("\n")
        for line in lines:
            for env_var in GIMME_ENV_VARS:
                if env_var in line:
                    goenv[env_var] = line[line.find(env_var) + len(env_var) + 1 : -1].strip('\'\"')
        ld_vars["GoVersion"] = go_version

    # extend PATH from gimme with the one from get_build_flags
    if "PATH" in os.environ and "PATH" in goenv:
        goenv["PATH"] += ":" + os.environ["PATH"]
    env.update(goenv)

    ldflags += ' '.join(["-X '{name}={value}'".format(name=main + key, value=value) for key, value in ld_vars.items()])
    build_tags = get_default_build_tags(build="process-agent", arch=arch)

    ## secrets is not supported on windows because the process agent still runs as
    ## root.  No matter what `get_default_build_tags()` returns, take secrets out.
    if sys.platform == 'win32' and "secrets" in build_tags:
        build_tags.remove("secrets")

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


@task
def build_dev_image(ctx, image=None, push=False, base_image="datadog/agent:latest"):
    """
    Build a dev image of the process-agent based off an existing datadog-agent image

    image: the image name used to tag the image
    push: if true, run a docker push on the image
    base_image: base the docker image off this already build image (default: datadog/agent:latest)
    """
    if image is None:
        raise Exit(message="image was not specified")

    with TempDir() as docker_context:
        ctx.run("cp tools/ebpf/Dockerfiles/Dockerfile-process-agent-dev {to}".format(to=docker_context + "/Dockerfile"))

        ctx.run("cp bin/process-agent/process-agent {to}".format(to=docker_context + "/process-agent"))

        ctx.run("cp bin/system-probe/system-probe {to}".format(to=docker_context + "/system-probe"))
        ctx.run("cp bin/agent/agent {to}".format(to=docker_context + "/agent"))
        ctx.run("cp pkg/ebpf/bytecode/build/*.o {to}".format(to=docker_context))

        with ctx.cd(docker_context):
            # --pull in the build will force docker to grab the latest base image
            ctx.run(
                "docker build --pull --tag {image} --build-arg AGENT_BASE={base_image} .".format(
                    image=image, base_image=base_image
                )
            )

    if push:
        ctx.run("docker push {image}".format(image=image))


class TempDir:
    def __enter__(self):
        self.fname = tempfile.mkdtemp()
        print("created tempdir: {name}".format(name=self.fname))
        return self.fname

    def __exit__(self, exception_type, exception_value, traceback):
        print("deleting tempdir: {name}".format(name=self.fname))
        shutil.rmtree(self.fname)
