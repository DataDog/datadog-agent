from __future__ import annotations

import sys
from pathlib import Path
from typing import TYPE_CHECKING, cast

import yaml
from invoke.context import Context
from invoke.runners import Result

from tasks.kernel_matrix_testing.tool import full_arch, info, warn
from tasks.kernel_matrix_testing.vars import arch_mapping
from tasks.pipeline import GitlabYamlLoader

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import Arch, ArchOrLocal, PathOrStr


CONTAINER_AGENT_PATH = "/tmp/datadog-agent"


def get_build_image_suffix_and_version() -> tuple[str, str]:
    gitlab_ci_file = Path(__file__).parent.parent.parent / ".gitlab-ci.yml"
    with open(gitlab_ci_file) as f:
        ci_config = yaml.load(f, Loader=GitlabYamlLoader())

    ci_vars = ci_config['variables']
    return ci_vars['DATADOG_AGENT_BUILDIMAGES_SUFFIX'], ci_vars['DATADOG_AGENT_BUILDIMAGES']


class CompilerImage:
    def __init__(self, ctx: Context, arch: Arch):
        self.ctx = ctx
        self.arch: Arch = arch

    @property
    def name(self):
        return f"kmt-compiler-{self.arch}"

    @property
    def image(self):
        suffix, version = get_build_image_suffix_and_version()
        image_base = "486234852809.dkr.ecr.us-east-1.amazonaws.com/ci/datadog-agent-buildimages/system-probe"
        image_arch = "x64" if self.arch == "x86_64" else "arm64"

        return f"{image_base}_{image_arch}{suffix}:{version}"

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

    def exec(self, cmd: str, user="compiler", verbose=True, run_dir: PathOrStr | None = None):
        if run_dir:
            cmd = f"cd {run_dir} && {cmd}"

        self.ensure_running()
        return self.ctx.run(f"docker exec -u {user} -i {self.name} bash -c \"{cmd}\"", hide=(not verbose))

    def stop(self) -> Result:
        res = self.ctx.run(f"docker rm -f $(docker ps -aqf \"name={self.name}\")")
        return cast('Result', res)  # Avoid mypy error about res being None

    def start(self) -> None:
        if self.is_running:
            self.stop()

        # Check if the image exists
        res = self.ctx.run(f"docker image inspect {self.image}", hide=True, warn=True)
        if res is None or not res.ok:
            info(f"[!] Image {self.image} not found, logging in and pulling...")
            self.ctx.run(
                "aws-vault exec sso-build-stable-developer -- aws ecr --region us-east-1 get-login-password | docker login --username AWS --password-stdin 486234852809.dkr.ecr.us-east-1.amazonaws.com"
            )
            self.ctx.run(f"docker pull {self.image}")

        self.ctx.run(
            f"docker run -d --restart always --name {self.name} --mount type=bind,source=./,target={CONTAINER_AGENT_PATH} {self.image} sleep \"infinity\""
        )

        # Due to permissions issues, we do not want to compile with the root user in the Docker image. We create a user
        # inside there with the same UID and GID as the current user
        uid = cast('Result', self.ctx.run("id -u")).stdout.rstrip()
        gid = cast('Result', self.ctx.run("id -g")).stdout.rstrip()

        if uid == 0:
            # If we're starting the compiler as root, we won't be able to create the compiler user
            # and we will get weird failures later on, as the user 'compiler' won't exist in the container
            raise ValueError("Cannot start compiler as root, we need to run as a non-root user")

        # Now create the compiler user with same UID and GID as the current user
        self.exec(f"getent group {gid} || groupadd -f -g {gid} compiler", user="root")
        self.exec(f"getent passwd {uid} || useradd -m -u {uid} -g {gid} compiler", user="root")

        if sys.platform != "darwin":  # No need to change permissions in MacOS
            self.exec(
                f"chown {uid}:{gid} {CONTAINER_AGENT_PATH} && chown -R {uid}:{gid} {CONTAINER_AGENT_PATH}", user="root"
            )

        self.exec("chmod a+rx /root", user="root")  # Some binaries will be in /root and need to be readable
        self.exec("apt install sudo", user="root")
        self.exec("usermod -aG sudo compiler && echo 'compiler ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers", user="root")
        self.exec("echo conda activate ddpy3 >> /home/compiler/.bashrc", user="compiler")
        self.exec(f"install -d -m 0777 -o {uid} -g {uid} /go", user="root")


def get_compiler(ctx: Context, arch: ArchOrLocal):
    return CompilerImage(ctx, full_arch(arch))


def all_compilers(ctx: Context):
    return [get_compiler(ctx, arch) for arch in arch_mapping.values()]
