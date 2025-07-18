"""
Utility functions for the setup requirements
"""

import re
import tempfile
from pathlib import Path
from typing import Any

from invoke.context import Context

from tasks.kernel_matrix_testing.setup.requirement import RequirementState
from tasks.libs.common.status import Status


def ensure_options_in_config(
    ctx: Context, config_file: Path, options: dict[str, Any], change: bool, write_with_sudo: bool = False
) -> list[str]:
    """
    Ensure that the given options are present in the config file.

    Args:
        ctx: invoke context
        config_file: Path to the config file
        options: dict of options to ensure
        change: if True, the config file will be changed
        write_with_sudo: if True, the config file will be written with sudo

    Returns:
        list[str]: list of option names that were not correct in the file
    """
    with open(config_file) as f:
        content = f.read().splitlines()

    incorrect_options, updated_lines = _patch_config_lines(content, options, change)

    if len(incorrect_options) > 0 and change:
        with tempfile.NamedTemporaryFile(delete_on_close=False) as temp_file:
            temp_file.write("\n".join(updated_lines).encode("utf-8"))
            temp_file.close()

            sudo = "sudo " if write_with_sudo else ""
            ctx.run(f"{sudo}mv {temp_file.name} {config_file}")

    return incorrect_options


def _patch_config_lines(lines: list[str], options: dict[str, Any], change: bool) -> tuple[list[str], list[str]]:
    """
    Patch the config lines to ensure the given options are present. Split from ensure_options_in_config to make it easier to test.

    Args:
        lines: list of lines to patch
        options: dict of options to ensure
        change: if True, the config file will be changed

    Returns:
        tuple[list[str], list[str]]: list of incorrect options and list of updated lines
    """
    incorrect_options: list[str] = []
    updated_lines: list[str] = []

    line_regexes = {option: re.compile(f"^{option} *=") for option in options.keys()}
    comment_regexes = {option: re.compile(f"^# *{option} *=") for option in options.keys()}

    for line in lines:
        changed_line = False

        for option in options.keys():
            if line_regexes[option].match(line):
                configured_value = line.split("=")[1].strip()
                formatted_value = _get_formatted_value(options[option])
                if configured_value != formatted_value:
                    incorrect_options.append(option)
                    if change:
                        updated_lines.append(f"{option} = {formatted_value}")
                        changed_line = True
                    break
            elif comment_regexes[option].match(line):
                incorrect_options.append(option)
                if change:
                    updated_lines.append(f"{option} = {_get_formatted_value(options[option])}")
                    changed_line = True
                break

        if not changed_line:
            updated_lines.append(line)

    return incorrect_options, updated_lines


def _get_formatted_value(value: Any) -> str:
    if isinstance(value, str):
        return f"\"{value}\""

    return str(value)


def check_launchctl_service(
    ctx: Context, service_name: str, fix: bool, service_install_file: str | None = None, run_at_boot: bool = True
) -> RequirementState:
    """Checks that a launchctl macos service is loaded, started and enabled

    Args:
        ctx: invoke context
        service_name: name of the launchctl service
        fix: if True, the service will be loaded, started and enabled.
        service_install_file: path to the service install file. If not provided, the service will be installed from this file if it is not loaded.

    Returns:
        RequirementState: status of the service
    """
    launchctl_data = ctx.run(f"sudo launchctl print {service_name}", warn=True)
    tried_to_load = False

    # If the service is not loaded, try to load it once (if we can)
    while launchctl_data is None or not launchctl_data.ok:
        if not fix or service_install_file is None or tried_to_load:  # Do not retry reloading
            return RequirementState(
                Status.FAIL,
                f"launchctl service {service_name} not loaded",
                fixable=service_install_file is not None,
            )

        try:
            tried_to_load = True
            ctx.run(f"sudo launchctl load -w {service_install_file}")
        except Exception as e:
            return RequirementState(Status.FAIL, f"Failed to load launchctl service: {e}")

        launchctl_data = ctx.run(f"sudo launchctl print {service_name}", warn=True)

    service_info = launchctl_data.stdout

    if "runatload" not in service_info:
        if not fix:
            return RequirementState(
                Status.FAIL, f"launchctl service {service_name} not set to run at load", fixable=True
            )

        try:
            ctx.run(f"sudo launchctl enable {service_name}")
        except Exception as e:
            return RequirementState(Status.FAIL, f"Failed to enable launchctl service: {e}")

    if run_at_boot and "state = running" not in service_info:
        if not fix:
            return RequirementState(Status.FAIL, f"launchctl service {service_name} not running", fixable=True)

        try:
            ctx.run(f"sudo launchctl start {service_name}")
        except Exception as e:
            return RequirementState(Status.FAIL, f"Failed to start launchctl service: {e}")

    return RequirementState(Status.OK, f"launchctl service {service_name} is loaded, started and enabled")
