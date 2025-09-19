import getpass
import shutil

from invoke.context import Context

from tasks.libs.common.status import Status
from tasks.libs.types.arch import Arch

from ..tool import is_root
from .common import Docker
from .requirement import Requirement, RequirementState
from .utils import UbuntuPackageManager, UbuntuSnapPackageManager


def get_requirements() -> list[Requirement]:
    return [UserInDockerGroup(), LinuxBasePackages()]


class UserInDockerGroup(Requirement):
    dependencies: list[type[Requirement]] = [Docker]

    def check(self, ctx: Context, _: bool) -> RequirementState:
        ret = ctx.run(
            "cat /proc/$$/status | grep '^Groups:' | grep $(cat /etc/group | grep 'docker:' | cut -d ':' -f 3)",
            warn=True,
        )
        if ret is None or not ret.ok:
            return RequirementState(
                Status.FAIL,
                f"User '{getpass.getuser()}' is not in docker group. Please resolve this https://docs.docker.com/engine/install/linux-postinstall/",
            )

        return RequirementState(Status.OK, "User is in docker group.")


class LinuxBasePackages(Requirement):
    def _check_aws_vault(self, ctx: Context, fix: bool) -> RequirementState:
        # aws-vault requires manual installation
        if shutil.which("aws-vault") is not None:
            return RequirementState(Status.OK, "aws-vault is installed.")

        if not fix:
            return RequirementState(Status.FAIL, "aws-vault is not installed.", fixable=True)

        arch = Arch.local().go_arch
        download_url = f"https://github.com/99designs/aws-vault/releases/download/v7.2.0/aws-vault-linux-{arch}"

        ctx.run(f"curl -L {download_url} -o aws-vault")
        ctx.run("chmod +x aws-vault")
        sudo = "sudo " if not is_root() else ""
        ctx.run(f"{sudo}mv aws-vault /usr/local/bin/")

        return RequirementState(Status.OK, "aws-vault installed.")

    def check(self, ctx: Context, fix: bool) -> list[RequirementState]:
        snap_packages: list[str] = []
        apt_packages: list[str] = ["xsltproc"]

        # These packages might have alternative means of installation, so
        # check if the command exists rather than checking for the package
        if shutil.which("aws") is None:
            snap_packages.append("aws-cli")

        # Not on the default repos, so we have to use snap
        snap_pkg_state = UbuntuSnapPackageManager(ctx, classic=True).check(snap_packages, fix)
        apt_pkg_state = UbuntuPackageManager(ctx).check(apt_packages, fix)

        return [snap_pkg_state, apt_pkg_state, self._check_aws_vault(ctx, fix)]
