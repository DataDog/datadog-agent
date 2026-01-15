import platform
from typing import cast

from invoke.context import Context
from invoke.runners import Result

from tasks.libs.common.status import Status

from .requirement import Requirement, RequirementState
from .utils import PipPackageManager


def get_requirements() -> list[Requirement]:
    return [LibvirtPython()]


class LibvirtPython(Requirement):
    """
    Check that the libvirt-python package is installed. This package does not get installed
    in the same as the other Python requirements because its version needs to match
    the version of libvirt installed on the system.
    """

    def __init__(self):
        super().__init__()

        if platform.system() == "Darwin":
            from .mac_localvms import MacPackages

            self.dependencies: list[type[Requirement]] = [MacPackages]
        else:
            from .linux_localvms import LinuxPackages

            self.dependencies: list[type[Requirement]] = [LinuxPackages]

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        res = cast(Result, ctx.run("libvirtd --version"))

        out_parts = res.stdout.strip().split("\n")[0].split(" ")
        if len(out_parts) != 3:
            return RequirementState(
                Status.FAIL,
                f"unexpected output from libvirtd --version: {res.stdout.strip()}",
            )

        libvirt_version = out_parts[2]

        return PipPackageManager(ctx).check([f"libvirt-python=={libvirt_version}"], fix)
