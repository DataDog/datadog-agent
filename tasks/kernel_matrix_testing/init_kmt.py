from __future__ import annotations

import getpass
import os
import shutil
import sys
from pathlib import Path
from typing import TYPE_CHECKING

from invoke.context import Context

from tasks.kernel_matrix_testing.compiler import get_compiler
from tasks.kernel_matrix_testing.config import ConfigManager
from tasks.kernel_matrix_testing.download import download_rootfs
from tasks.kernel_matrix_testing.kmt_os import get_kmt_os
from tasks.kernel_matrix_testing.tool import Exit, ask, info, is_root, warn
from tasks.libs.common.utils import is_installed

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import PathOrStr


def gen_ssh_key(ctx: Context, kmt_dir: PathOrStr):
    ctx.run(f"cp tasks/kernel_matrix_testing/ddvm_rsa {kmt_dir}")
    ctx.run(f"chmod 600 {kmt_dir}/ddvm_rsa")


def init_kernel_matrix_testing_system(
    ctx: Context, images: str | None = None, all_images: bool = False, remote_setup_only: bool = False
):
    kmt_os = get_kmt_os()

    info("[+] Installing OS-specific general requirements...")
    kmt_os.install_requirements(ctx)

    if sys.version_info >= (3, 12):
        resp = ask("Python 3.12+ is not tested yet with KMT, some packages might not be available. Continue? (y/N)? ")
        if resp.lower().strip() != "y":
            raise Exit("Aborted by user")

    # trigger install of dependencies
    if is_installed("dda"):
        ctx.run("dda inv --feat legacy-kernel-matrix-testing -- --help")
    else:
        reqs_file = Path(__file__).parent / "requirements.txt"
        ctx.run(f"pip3 install -r {reqs_file.absolute()}")

    if shutil.which("pulumi") is None:
        if Path("~/.pulumi/bin/pulumi").expanduser().exists():
            raise Exit("pulumi is installed in ~/.pulumi/bin/pulumi, but not in $PATH. Add it to $PATH.")

        ctx.run("curl -fsSL https://get.pulumi.com | sh")
        os.environ["PATH"] = f"{os.environ['PATH']}:{os.path.expanduser('~/.pulumi/bin')}"

        if shutil.which("pulumi") is None:
            raise Exit("pulumi not found in $PATH after installation")

        ctx.run("pulumi login --local")

    repo_root = Path(__file__).parent.parent.parent
    test_infra_dir = repo_root.parent / "test-infra-definitions"

    # incase the directory structure is not expected, try with the following layout.
    test_infra_alt = Path("~/go/src/github.com/DataDog/test-infra-definitions")

    if not test_infra_dir.is_dir() and test_infra_alt.is_dir():
        test_infra_dir = test_infra_alt

    if not test_infra_dir.is_dir():
        resp = ask(
            f"test-infra-definitions directory not found in {test_infra_dir.absolute()}. Clone the repository? (y/N)? "
        )
        if resp.lower().strip() != "y":
            raise Exit("Aborted by user")

        ctx.run(f"git clone git@github.com:DataDog/test-infra-definitions.git {test_infra_dir}")

    info("[+] Installing pulumi plugins")
    with ctx.cd(test_infra_dir):
        ctx.run("go mod download")
        ctx.run("PULUMI_CONFIG_PASSPHRASE=dummy pulumi --non-interactive plugin install")

    pulumi_test_cmd = "pulumi --non-interactive plugin ls"
    res = ctx.run(pulumi_test_cmd, warn=True)
    if res is None or not res.ok:
        raise Exit(
            f"Running {pulumi_test_cmd} failed, check that the installation is correct (see tasks/kernel_matrix_testing/README.md)"
        )

    if not remote_setup_only:
        if shutil.which("libvirtd") is None:
            if Path("/opt/homebrew/sbin/libvirtd").exists():
                raise Exit(
                    "libvirtd is installed in /opt/homebrew/sbin/libvirtd, but not in $PATH. /opt/homebrew/sbin should be part of your $PATH variable"
                )
            else:
                raise Exit("libvirtd not found in $PATH, did you run tasks/kernel_matrix_testing/env-setup.sh?")

        info("[+] OS-specific setup")
        kmt_os.init_local(ctx)

    info("[+] Creating KMT directories")
    sudo = "sudo" if not is_root() else ""
    user = getpass.getuser()
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.kmt_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.packages_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.stacks_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.libvirt_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.rootfs_dir}")
    ctx.run(f"{sudo} install -d -m 0755 -g {kmt_os.libvirt_group} -o {user} {kmt_os.shared_dir}")

    # download dependencies
    if not remote_setup_only:
        if all_images or images:
            info("[+] Downloading VM images")
            download_rootfs(ctx, kmt_os.rootfs_dir, "system-probe", arch=None, images=None if all_images else images)
    else:
        warn("[-] Skipping local VM image download since remote only setup is selected")

    # Copy the SSH key we use to connect
    gen_ssh_key(ctx, kmt_os.kmt_dir)

    # build docker compile image
    info("[+] Starting compiler image")
    kmt_os.assert_user_in_docker_group(ctx)
    info(f"[+] User '{getpass.getuser()}' in group 'docker'")

    get_compiler(ctx).start()

    cm = ConfigManager()
    cm.config["setup"] = "remote" if remote_setup_only else "full"
    cm.save()
