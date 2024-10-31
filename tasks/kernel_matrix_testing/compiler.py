from __future__ import annotations

import json
import os
import sys
from pathlib import Path
from typing import TYPE_CHECKING, cast

import yaml
from invoke.context import Context
from invoke.runners import Result

from tasks.kernel_matrix_testing.tool import Exit, info, warn
from tasks.libs.ciproviders.gitlab_api import ReferenceTag
from tasks.libs.types.arch import ARCH_AMD64, ARCH_ARM64, Arch

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import PathOrStr


CONTAINER_AGENT_PATH = "/tmp/datadog-agent"

AMD64_DEBIAN_KERNEL_HEADERS_URL = "http://deb.debian.org/debian-security/pool/updates/main/l/linux-5.10/linux-headers-5.10.0-0.deb10.28-amd64_5.10.209-2~deb10u1_amd64.deb"
ARM64_DEBIAN_KERNEL_HEADERS_URL = "http://deb.debian.org/debian-security/pool/updates/main/l/linux-5.10/linux-headers-5.10.0-0.deb10.28-arm64_5.10.209-2~deb10u1_arm64.deb"

DOCKER_REGISTRY = "486234852809.dkr.ecr.us-east-1.amazonaws.com"
DOCKER_IMAGE_BASE = f"{DOCKER_REGISTRY}/ci/datadog-agent-buildimages/system-probe"


def get_build_image_suffix_and_version() -> tuple[str, str]:
    gitlab_ci_file = Path(__file__).parent.parent.parent / ".gitlab-ci.yml"
    yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
    with open(gitlab_ci_file) as f:
        ci_config = yaml.safe_load(f)

    ci_vars = ci_config['variables']
    return ci_vars['DATADOG_AGENT_SYSPROBE_BUILDIMAGES_SUFFIX'], ci_vars['DATADOG_AGENT_SYSPROBE_BUILDIMAGES']


def get_docker_image_name(ctx: Context, container: str) -> str:
    res = ctx.run(f"docker inspect \"{container}\"", hide=True)
    if res is None or not res.ok:
        raise ValueError(f"Could not get {container} info")

    data = json.loads(res.stdout)
    return data[0]["Config"]["Image"]


def has_docker_auth_helpers() -> bool:
    docker_config = Path("~/.docker/config.json").expanduser()
    if not docker_config.exists():
        return False

    try:
        with open(docker_config) as f:
            config = json.load(f)
    except json.JSONDecodeError:
        # Invalid JSON (or empty file), we don't have the helper
        return False

    return DOCKER_REGISTRY in config.get("credHelpers", {})


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

        return f"{DOCKER_IMAGE_BASE}_{self.arch.ci_arch}{suffix}:{version}"

    def _check_container_exists(self, allow_stopped=False):
        if self.ctx.config.run["dry"]:
            warn(f"[!] Dry run, not checking if compiler {self.name} is running")
            return True

        args = "a" if allow_stopped else ""
        res = self.ctx.run(f"docker ps -{args}qf \"name={self.name}\"", hide=True)
        if res is not None and res.ok:
            return res.stdout.rstrip() != ""
        return False

    @property
    def is_running(self):
        return self._check_container_exists(allow_stopped=False)

    @property
    def is_loaded(self):
        return self._check_container_exists(allow_stopped=True)

    def ensure_running(self):
        if not self.is_running:
            info(f"[*] Compiler for {self.arch} not running, starting it...")
            try:
                self.start()
            except Exception as e:
                raise Exit(f"Failed to start compiler for {self.arch}: {e}") from e

    def ensure_version(self):
        if not self.is_loaded:
            return  # Nothing to do if the container is not loaded

        image_used = get_docker_image_name(self.ctx, self.name)
        if image_used != self.image:
            warn(f"[!] Running compiler image {image_used} is different from the expected {self.image}, will restart")
            self.start()

    def exec(self, cmd: str, user="compiler", verbose=True, run_dir: PathOrStr | None = None, allow_fail=False):
        if run_dir:
            cmd = f"cd {run_dir} && {cmd}"

        self.ensure_running()

        # Set FORCE_COLOR=1 so that termcolor works in the container
        return self.ctx.run(
            f"docker exec -u {user} -i -e FORCE_COLOR=1 {self.name} bash -c \"{cmd}\"",
            hide=(not verbose),
            warn=allow_fail,
        )

    def stop(self) -> Result:
        res = self.ctx.run(f"docker rm -f $(docker ps -aqf \"name={self.name}\")")
        return cast('Result', res)  # Avoid mypy error about res being None

    def start(self) -> None:
        if self.is_loaded:
            self.stop()

        # Check if the image exists
        res = self.ctx.run(f"docker image inspect {self.image}", hide=True, warn=True)
        if res is None or not res.ok:
            info(f"[!] Image {self.image} not found, logging in and pulling...")

            if has_docker_auth_helpers():
                # With ddtool helpers (installed with ddtool auth helpers install), docker automatically
                # pulls credentials from ddtool, and we require the aws-vault context to pull
                docker_pull_auth = "aws-vault exec sso-build-stable-developer -- "
            else:
                # Without the helpers, we need to get the password and login manually to docker
                self.ctx.run(
                    "aws-vault exec sso-build-stable-developer -- aws ecr --region us-east-1 get-login-password | docker login --username AWS --password-stdin 486234852809.dkr.ecr.us-east-1.amazonaws.com"
                )
                docker_pull_auth = ""

            self.ctx.run(f"{docker_pull_auth}docker pull {self.image}")

        res = self.ctx.run(
            f"docker run -d --restart always --name {self.name} "
            f"--mount type=bind,source={os.getcwd()},target={CONTAINER_AGENT_PATH} "
            f"{self.image} sleep \"infinity\"",
            warn=True,
        )
        if res is None or not res.ok:
            raise ValueError(f"Failed to start compiler container {self.name}")

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

        # Install all requirements except for libvirt ones (they won't build in the compiler and are not needed)
        self.exec(
            f"cat {CONTAINER_AGENT_PATH}/tasks/kernel_matrix_testing/requirements.txt | grep -v libvirt | xargs pip install ",
            user="compiler",
        )

        self.prepare_for_cross_compile()

    def ensure_ready_for_cross_compile(self):
        res = self.exec("test -f /tmp/cross-compile-ready", user="root", allow_fail=True)
        if res is None or not res.ok:
            info("[*] Compiler image not ready for cross-compilation, preparing...")
            self.prepare_for_cross_compile()

    def prepare_for_cross_compile(self):
        target = ARCH_AMD64 if self.arch == ARCH_ARM64 else ARCH_ARM64

        # Hardcoded links to the header packages for each architecture. Why do this and not have something more automated?
        # 1. While right now the URLs are similar and we'd only need a single link with variable replacement, this might
        #    change as the repository layout is not under our control.
        # 2. Automatic detection of these URLs is not direct (querying the package repo APIs is not trivial) and we'd need some
        #    level of hard-coding some URLs or assumptions anyways.
        # 3. Even if someone forgets to update these URLs, it's not a big deal, as we're building inside of a Docker image which will
        #    likely have a different kernel than the target system where the built eBPF files are going to run anyways.
        header_package_urls: dict[Arch, str] = {
            ARCH_AMD64: AMD64_DEBIAN_KERNEL_HEADERS_URL,
            ARCH_ARM64: ARM64_DEBIAN_KERNEL_HEADERS_URL,
        }

        header_package_path = "/tmp/headers.deb"
        self.exec(f"wget -O {header_package_path} {header_package_urls[target]}")

        # Uncompress the package in the root directory, so that we have access to the headers
        # We cannot install because the architecture will not match
        # Extract into a .tar file and then use tar to extract the contents to avoid issues
        # with dpkg-deb not respecting symlinks.
        self.exec(f"dpkg-deb --fsys-tarfile {header_package_path} > {header_package_path}.tar", user="root")
        self.exec(f"tar -h -xf {header_package_path}.tar -C /", user="root")

        # Install the corresponding arch compilers
        self.exec(f"apt update && apt install -y gcc-{target.gcc_arch.replace('_', '-')}-linux-gnu", user="root")
        self.exec("touch /tmp/cross-compile-ready")  # Signal that we're ready for cross-compilation


def get_compiler(ctx: Context):
    cc = CompilerImage(ctx, Arch.local())
    cc.ensure_version()
    cc.ensure_running()

    return cc
