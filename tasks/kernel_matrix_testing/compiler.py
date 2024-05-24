from __future__ import annotations

import sys
from pathlib import Path
from typing import TYPE_CHECKING, cast

import yaml
from invoke.context import Context
from invoke.runners import Result

from tasks.kernel_matrix_testing.tool import info, warn
from tasks.libs.types.arch import ARCH_AMD64, ARCH_ARM64, Arch
from tasks.pipeline import GitlabYamlLoader

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import PathOrStr


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
        return f"kmt-compiler-{self.arch.name}"

    @property
    def image(self):
        suffix, version = get_build_image_suffix_and_version()
        image_base = "486234852809.dkr.ecr.us-east-1.amazonaws.com/ci/datadog-agent-buildimages/system-probe"
        image_arch = "x64" if self.arch == ARCH_AMD64 else "arm64"

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

    def exec(self, cmd: str, user="compiler", verbose=True, run_dir: PathOrStr | None = None, warn=False):
        if run_dir:
            cmd = f"cd {run_dir} && {cmd}"

        self.ensure_running()
        return self.ctx.run(f"docker exec -u {user} -i {self.name} bash -c \"{cmd}\"", hide=(not verbose), warn=warn)

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

        self.prepare_for_cross_compile()

    def ensure_ready_for_cross_compile(self):
        res = self.exec("test -f /tmp/cross-compile-ready", user="root", warn=True)
        if res is not None and not res.ok:
            info("[*] Compiler image not ready for cross-compilation, preparing...")
            self.prepare_for_cross_compile()

    def prepare_for_cross_compile(self):
        target: Arch
        if self.arch == ARCH_ARM64:
            target = ARCH_AMD64
        else:
            target = ARCH_ARM64

        # Hardcoded links to the header packages for each architecture. Reasons:
        # 1. While right now they're similar and we'd only need a single link, in other cases they have different formats
        # 2. Automated retrieval of these URLs is not easy
        # 3. As we're cross-compiling, even if these become outdated with respect to the kernel used in the build image, we don't
        #    care that much as the kernel version is not going to match anyways, we're not running in the local system.
        header_package_urls: dict[Arch, str] = {
            ARCH_AMD64: "http://deb.debian.org/debian-security/pool/updates/main/l/linux-5.10/linux-headers-5.10.0-0.deb10.28-amd64_5.10.209-2~deb10u1_amd64.deb",
            ARCH_ARM64: "http://deb.debian.org/debian-security/pool/updates/main/l/linux-5.10/linux-headers-5.10.0-0.deb10.28-arm64_5.10.209-2~deb10u1_arm64.deb",
        }

        header_package_path = "/tmp/headers.deb"
        self.exec(f"wget -O {header_package_path} {header_package_urls[target]}")

        # Uncompress the package in the root directory, so that we have access to the headers
        # We cannot install because the architecture will not match
        self.exec(f"dpkg-deb -x {header_package_path} /", user="root")

        # Install the corresponding arch compilers
        self.exec(f"apt update && apt install -y gcc-{target.gcc_arch.replace('_', '-')}-linux-gnu", user="root")
        self.exec("touch /tmp/cross-compile-ready")  # Signal that we're ready for cross-compilation


def get_compiler(ctx: Context):
    return CompilerImage(ctx, Arch.local())
