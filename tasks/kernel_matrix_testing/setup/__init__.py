import sys
from types import ModuleType

from invoke.context import Context

from tasks.kernel_matrix_testing.setup.requirement import Requirement, RequirementState, summarize_requirement_states
from tasks.kernel_matrix_testing.tool import info, is_root
from tasks.libs.common.status import Status


def get_requirements(remote_setup_only: bool):
    from . import common, linux, linux_localvms, mac, mac_localvms

    requirements: list[Requirement] = []

    requirements += _get_requirements_from_module(common)

    if sys.platform == "linux":
        requirements += _get_requirements_from_module(linux)
    elif sys.platform == "darwin":
        requirements += _get_requirements_from_module(mac)

    if not remote_setup_only:
        if sys.platform == "linux":
            requirements += _get_requirements_from_module(linux_localvms)
        elif sys.platform == "darwin":
            requirements += _get_requirements_from_module(mac_localvms)

    return requirements


def _get_requirements_from_module(module: ModuleType) -> list[Requirement]:
    # Get all classes in the module that inherit from Requirement
    return [
        cls()
        for cls in module.__dict__.values()
        if isinstance(cls, type) and issubclass(cls, Requirement) and cls != Requirement
    ]


def check_requirements(
    ctx: Context, requirements: list[Requirement], fix: bool, echo: bool = True, verbose: bool = False
) -> bool:
    if echo:
        info("Checking requirements...")

    any_fail = False

    if not verbose:
        old_hide = ctx.config["run"]["hide"]
        ctx.config["run"]["hide"] = True

    if not is_root():
        print("Some checks require root privileges, asking for sudo now:")
        ctx.run("sudo true")

    for requirement in requirements:
        name = requirement.__class__.__name__
        if echo:
            print(f"{name} ...", end="\n" if verbose else "", flush=True)

        try:
            result = requirement.check(ctx, fix)
        except Exception as e:
            result = RequirementState(Status.FAIL, f"Exception checking requirement: {e}")

        state = summarize_requirement_states(result)

        if echo:
            print(f"\r{name} {state}")

        if state.state == Status.FAIL:
            any_fail = True

    if not verbose:
        ctx.config["run"]["hide"] = old_hide

    return any_fail
