from invoke.context import Context
from invoke.exceptions import Exit

from tasks.e2e_framework import tool

from .tool import error, get_stack_name, get_stack_name_prefix, info


def destroy(
    ctx: Context,
    *,
    scenario_name: str,
    config_path: str | None = None,
    stack: str | None = None,
):
    """
    Destroy an environment
    """
    from pydantic_core._pydantic_core import ValidationError

    from tasks.e2e_framework import config

    full_stack_name = get_stack_name(stack, scenario_name)

    short_stack_names, full_stack_names = _get_existing_stacks(tool.pulumi_stack_names(ctx, config_path=config_path))
    if len(short_stack_names) == 0:
        info("No stack to destroy")
        return

    try:
        config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}:{e}") from e

    if stack is not None:
        if stack in short_stack_names:
            full_stack_name = f"{get_stack_name_prefix()}{stack}"
        else:
            error(f"Unknown stack '{stack}'")
            full_stack_name = None
    else:
        if full_stack_name not in full_stack_names:
            error(f"Unknown stack '{full_stack_name}'")
            full_stack_name = None

    if full_stack_name is None:
        error("Run this command with '--stack-name MY_STACK_NAME'. Available stacks are:")
        for stack_name in short_stack_names:
            error(f" {stack_name}")
    else:
        args = "destroy --remove --yes --skip-preview"
        ret = tool.run_pulumi(ctx, args, stack=full_stack_name, config_path=config_path, pty=True, warn=True)
        if ret is not None and ret.exited != 0:
            # run with refresh on first destroy attempt failure
            tool.run_pulumi(ctx, f"{args} --refresh", stack=full_stack_name, config_path=config_path, pty=True)


def _get_existing_stacks(stack_names: list[str]) -> tuple[list[str], list[str]]:
    """
    Split the stacks belonging to the current user out of `stack_names`, as both their
    short (prefix stripped) and full names.
    """
    stacks: list[str] = []
    full_stacks: list[str] = []
    stack_name_prefix = get_stack_name_prefix()
    for stack_name in stack_names:
        if stack_name.startswith(stack_name_prefix):
            full_stacks.append(stack_name)
            stacks.append(stack_name[len(stack_name_prefix) :])
    return stacks, full_stacks
