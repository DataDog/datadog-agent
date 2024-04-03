from __future__ import annotations

import os
import sys
from typing import TYPE_CHECKING, Optional, cast

from invoke.context import Context
from invoke.runners import Result

from tasks.kernel_matrix_testing.tool import info

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import PathOrStr


CONTAINER_AGENT_PATH = "/tmp/datadog-agent"


def compiler_built(ctx: Context):
    res = ctx.run("docker images kmt:compile | grep -v REPOSITORY | grep kmt", warn=True)
    return res is not None and res.ok


def docker_exec(ctx: Context, cmd: str, user="compiler", verbose=True, run_dir: Optional[PathOrStr] = None):
    if run_dir:
        cmd = f"cd {run_dir} && {cmd}"

    if not compiler_running(ctx):
        info("[*] Compiler not running, starting it...")
        start_compiler(ctx)

    ctx.run(f"docker exec -u {user} -i kmt-compiler bash -c \"{cmd}\"", hide=(not verbose))


def start_compiler(ctx: Context):
    if not compiler_built(ctx):
        build_compiler(ctx)

    if compiler_running(ctx):
        ctx.run("docker rm -f $(docker ps -aqf \"name=kmt-compiler\")")

    ctx.run(
        f"docker run -d --restart always --name kmt-compiler --mount type=bind,source=./,target={CONTAINER_AGENT_PATH} kmt:compile sleep \"infinity\""
    )

    uid = cast('Result', ctx.run("id -u")).stdout.rstrip()
    gid = cast('Result', ctx.run("id -g")).stdout.rstrip()
    docker_exec(ctx, f"getent group {gid} || groupadd -f -g {gid} compiler", user="root")
    docker_exec(ctx, f"getent passwd {uid} || useradd -m -u {uid} -g {gid} compiler", user="root")

    if sys.platform != "darwin":  # No need to change permissions in MacOS
        docker_exec(
            ctx, f"chown {uid}:{gid} {CONTAINER_AGENT_PATH} && chown -R {uid}:{gid} {CONTAINER_AGENT_PATH}", user="root"
        )

    docker_exec(ctx, "apt install sudo", user="root")
    docker_exec(ctx, "usermod -aG sudo compiler && echo 'compiler ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers", user="root")
    docker_exec(ctx, f"install -d -m 0777 -o {uid} -g {uid} /go", user="root")


def compiler_running(ctx: Context):
    # symlink working directory to /tmp/datadog-agent
    # This is done so that the DWARF lineinfo inside the ebpf object files point to the correct source
    # code files on the host machine.
    if not (os.path.islink(CONTAINER_AGENT_PATH) and os.readlink(CONTAINER_AGENT_PATH) == os.getcwd()):
        os.symlink(os.getcwd(), CONTAINER_AGENT_PATH, target_is_directory=True)

    res = ctx.run("docker ps -aqf \"name=kmt-compiler\"")
    if res is not None and res.ok:
        return res.stdout.rstrip() != ""
    return False


def build_compiler(ctx: Context):
    ctx.run("docker rm -f $(docker ps -aqf \"name=kmt-compiler\")", warn=True, hide=True)
    ctx.run("docker image rm kmt:compile", warn=True, hide=True)

    docker_build_args = [
        # Specify platform with --platform, even if we're running in ARM we want x86_64 images
        # Important because some packages needed by that image are not available in arm builds of debian
        "--platform",
        "linux/amd64",
    ]

    # Add build arguments (such as go version) from go.env
    with open("../datadog-agent-buildimages/go.env", "r") as f:
        for line in f:
            docker_build_args += ["--build-arg", line.strip()]

    docker_build_args_s = " ".join(docker_build_args)
    ctx.run(
        f"cd ../datadog-agent-buildimages && docker build {docker_build_args_s} -f system-probe_x64/Dockerfile -t kmt:compile ."
    )
