import sys

from invoke.context import Context

from tasks.libs.common.status import Status

from ..tool import info, is_root
from .requirement import Requirement, RequirementState, summarize_requirement_states


def get_requirements(
    remote_setup_only: bool, exclude_requirements: list[str] | None = None, only_requirements: list[str] | None = None
) -> list[Requirement]:
    from . import common, common_localvms, linux, linux_localvms, mac, mac_localvms

    requirements: list[Requirement] = []

    if sys.platform == "linux":
        requirements += linux.get_requirements()
    elif sys.platform == "darwin":
        requirements += mac.get_requirements()

    requirements += common.get_requirements()

    if not remote_setup_only:
        if sys.platform == "linux":
            requirements += linux_localvms.get_requirements()
        elif sys.platform == "darwin":
            requirements += mac_localvms.get_requirements()

        requirements += common_localvms.get_requirements()

    requirements = _topological_sort_requirements(requirements)

    if exclude_requirements is not None:
        requirements = [r for r in requirements if r.__class__.__name__ not in exclude_requirements]

    if only_requirements is not None:
        requirements = [r for r in requirements if r.__class__.__name__ in only_requirements]

    return requirements


def _topological_sort_requirements(requirements: list[Requirement]) -> list[Requirement]:
    """Topologically sorts the requirements based on their dependencies.

    This is used to ensure that the requirements are checked in the correct order,
    so that dependencies are checked before the requirements that depend on them.
    """
    req_by_class = {r.__class__: r for r in requirements}

    # Build adjacency list
    adj: dict[Requirement, list[Requirement]] = {r: [] for r in requirements}
    for r in requirements:
        for dep in r.dependencies or []:
            adj[req_by_class[dep]].append(r)

    in_degree = {r: 0 for r in requirements}
    for r in requirements:
        for dep in adj[r]:
            in_degree[dep] += 1

    # First select all requirements with no dependencies
    queue = [r for r in requirements if in_degree[r] == 0]
    sorted_reqs = []
    while len(queue) > 0:
        # Add this requirement to the sorted list
        r = queue.pop(0)
        sorted_reqs.append(r)

        # Check all the requirements that depend on this one
        for dep in adj[r]:
            # Reduce the in-degree of the dependency
            in_degree[dep] -= 1
            # If the in-degree is now 0, meaning all of its dependencies have been added to the list,
            # we can add it to the queue
            if in_degree[dep] == 0:
                queue.append(dep)

    # If the number of requirements in the sorted list is not the same as the number of requirements,
    # it means there is a cycle in the dependencies
    if len(sorted_reqs) != len(requirements):
        raise RuntimeError("Cycle detected in requirements dependencies")

    return sorted_reqs


def check_requirements(
    ctx: Context,
    requirements: list[Requirement],
    fix: bool,
    echo: bool = True,
    verbose: bool = False,
    show_flare_for_failures: bool = False,
) -> bool:
    if echo:
        info("Checking requirements...")

    any_fail = False

    if not verbose:
        old_hide = ctx.config["run"]["hide"]
        ctx.config["run"]["hide"] = True

    if not is_root():
        print(
            "Some checks require root privileges, authenticating 'sudo' now to avoid prompts later. It might ask for your password."
        )
        ctx.run("sudo true")

    requirement_succeded: dict[type[Requirement], bool] = {}

    for requirement in requirements:
        name = requirement.__class__.__name__
        if echo:
            print(f"{name} ...", end="\n" if verbose else "", flush=True)

        missing_requirements: list[str] = []
        for dep in requirement.dependencies or []:
            if dep not in requirement_succeded:
                raise RuntimeError(
                    f"Requirement {name} depends on {dep.__name__}, which has not been checked yet, this should not happen."
                )
            elif not requirement_succeded[dep]:
                missing_requirements.append(dep.__name__)

        if len(missing_requirements) == 0:
            try:
                result = requirement.check(ctx, fix)
            except Exception as e:
                result = RequirementState(Status.FAIL, f"Exception checking requirement: {e}")
        else:
            result = RequirementState(Status.FAIL, f"Failed prerequisites: {', '.join(missing_requirements)}")

        state = summarize_requirement_states(result)

        if echo:
            print(f"\r{name} {state}")

        if state.state == Status.FAIL:
            any_fail = True
            requirement_succeded[requirement.__class__] = False

            if show_flare_for_failures:
                flare = requirement.flare(ctx)
                if flare:
                    print(f"\n\t=== {name} diagnostic flare === ")
                    for key, value in flare.items():
                        print(f"\t\t-- {key}")
                        for line in value.splitlines():
                            print(f"\t\t | {line}")
                    print(f"\n\t=== End of {name} diagnostic flare ===\n")
        else:
            requirement_succeded[requirement.__class__] = True

    if not verbose:
        ctx.config["run"]["hide"] = old_hide

    return any_fail
