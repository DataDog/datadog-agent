from __future__ import annotations

import sys
from pathlib import Path
from typing import TYPE_CHECKING, Optional, cast

from invoke.context import Context
from invoke.runners import Result

from tasks.kernel_matrix_testing.tool import full_arch, info, warn
from tasks.kernel_matrix_testing.vars import arch_mapping

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import Arch, ArchOrLocal, PathOrStr


CONTAINER_AGENT_PATH = "/tmp/datadog-agent"


class CompilerImage:
    def __init__(self, ctx: Context, arch: Arch):
        self.ctx = ctx
        self.arch: Arch = arch

    @property
    def name(self):
        return f"kmt-compiler-{self.arch}"

    @property
    def image(self):
        return f"kmt:compile-{self.arch}"

    @property
    def is_built(self):
        res = self.ctx.run(f"docker images {self.image} | grep -v REPOSITORY | grep kmt", warn=True)
        return res is not None and res.ok

    def ensure_built(self):
        if not self.is_built:
            info(f"[*] Compiler image for {self.arch} not built, building it...")
            self.build()

    @property
    def is_running(self):
        if self.ctx.config.run["dry"]:
            warn(f"[!] Dry run, not checking if compiler {self.name} is running")
            return True

        res = self.ctx.run(f"docker ps -aqf \"name={self.name}\"", hide=True)
        if res is not None and res.ok:
            return res.stdout.rstrip() != ""
        return False

    def ensure_running(self):
        if not self.is_running:
            info(f"[*] Compiler for {self.arch} not running, starting it...")
            self.start()

    def exec(self, cmd: str, user="compiler", verbose=True, run_dir: Optional[PathOrStr] = None):
        if run_dir:
            cmd = f"cd {run_dir} && {cmd}"

        self.ensure_running()
        return self.ctx.run(f"docker exec -u {user} -i {self.name} bash -c \"{cmd}\"", hide=(not verbose))

    def build(self) -> Result:
        self.ctx.run(f"docker rm -f $(docker ps -aqf \"name={self.name}\")", warn=True, hide=True)
        self.ctx.run(f"docker image rm {self.image}", warn=True, hide=True)

        if self.arch == "x86_64":
            docker_platform = "linux/amd64"
            buildimages_arch = "x64"
        else:
            docker_platform = "linux/arm64"
            buildimages_arch = "arm64"

        docker_build_args = ["--platform", docker_platform]

        agent_path = Path(__file__).parent.parent.parent
        buildimages_path = (agent_path.parent / "datadog-agent-buildimages").resolve()

        if not buildimages_path.is_dir():
            raise FileNotFoundError(
                f"datadog-agent-buildimages not found at {buildimages_path}. Please clone the repository there to access compiler images"
            )

        # Add build arguments (such as go version) from go.env
        with open(buildimages_path / "go.env", "r") as f:
            for line in f:
                docker_build_args += ["--build-arg", line.strip()]

        docker_build_args_s = " ".join(docker_build_args)
        res = self.ctx.run(
            f"cd {buildimages_path} && docker build {docker_build_args_s} -f system-probe_{buildimages_arch}/Dockerfile -t {self.image} ."
        )
        return cast('Result', res)  # Avoid mypy error about res being None

    def stop(self) -> Result:
        res = self.ctx.run(f"docker rm -f $(docker ps -aqf \"name={self.name}\")")
        return cast('Result', res)  # Avoid mypy error about res being None

    def start(self) -> None:
        self.ensure_built()

        if self.is_running:
            self.stop()

        self.ctx.run(
            f"docker run -d --restart always --name {self.name} --mount type=bind,source=./,target={CONTAINER_AGENT_PATH} {self.image} sleep \"infinity\""
        )

        uid = cast('Result', self.ctx.run("id -u")).stdout.rstrip()
        gid = cast('Result', self.ctx.run("id -g")).stdout.rstrip()
        self.exec(f"getent group {gid} || groupadd -f -g {gid} compiler", user="root")
        self.exec(f"getent passwd {uid} || useradd -m -u {uid} -g {gid} compiler", user="root")

        if sys.platform != "darwin":  # No need to change permissions in MacOS
            self.exec(
                f"chown {uid}:{gid} {CONTAINER_AGENT_PATH} && chown -R {uid}:{gid} {CONTAINER_AGENT_PATH}", user="root"
            )

        self.exec("apt install sudo", user="root")
        self.exec("usermod -aG sudo compiler && echo 'compiler ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers", user="root")
        self.exec("echo conda activate ddpy3 >> /home/compiler/.bashrc", user="compiler")
        self.exec(f"install -d -m 0777 -o {uid} -g {uid} /go", user="root")


def get_compiler(ctx: Context, arch: ArchOrLocal):
    return CompilerImage(ctx, full_arch(arch))


def all_compilers(ctx: Context):
    return [get_compiler(ctx, arch) for arch in arch_mapping.values()]
