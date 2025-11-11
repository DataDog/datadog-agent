import shutil

from invoke.context import Context

from .requirement import Requirement, RequirementState
from .utils import MacosPackageManager


def get_requirements() -> list[Requirement]:
    return [MacBasePackages()]


class MacBasePackages(Requirement):
    def check(self, ctx: Context, fix: bool) -> RequirementState:
        packages: list[str] = []

        # These packages might have alternative means of installation, so
        # check if the command exists rather than checking for the package
        if shutil.which("aws-vault") is None:
            packages.append("aws-vault")

        if shutil.which("aws") is None:
            packages.append("awscli")

        return MacosPackageManager(ctx).check(packages, fix)
