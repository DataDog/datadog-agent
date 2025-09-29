"""
Utility functions for the setup requirements
"""

import getpass
import grp
import io
import pwd
import re
import sys
import tempfile
from pathlib import Path
from typing import cast

from invoke.context import Context
from invoke.runners import Result

from tasks.libs.common.status import Status

from ..tool import is_root
from .requirement import RequirementState


def ensure_options_in_config(
    ctx: Context,
    config_file: Path,
    options: dict[str, str | int],
    change: bool,
    write_with_sudo: bool = False,
    read_with_sudo: bool = False,
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
    if read_with_sudo:
        buffer = io.StringIO()
        res = ctx.run(f"sudo cat {config_file}", warn=True, out_stream=buffer)
        if res is None:
            raise RuntimeError(f"Failed to read config file {config_file}, unknown error")
        elif not res.ok:
            raise RuntimeError(f"Failed to read config file {config_file}: {res.stderr}")

        content = buffer.getvalue().strip().splitlines()
    else:
        # If without sudo, we can just read the file. This makes it easier to test
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


def _patch_config_lines(lines: list[str], options: dict[str, str | int], change: bool) -> tuple[list[str], list[str]]:
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


def _get_formatted_value(value: str | int) -> str:
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


def check_directories(
    ctx: Context, dirs: list[Path], fix: bool, user: str, group: str, mode: int
) -> list[RequirementState]:
    """
    Check that the given directories exist and have the correct permissions.

    Args:
        ctx: invoke context
        dirs: list of paths to ensure
        fix: if True, inexistent directories will be created, directories with incorrect permissions or ownership will be fixed
        user: user name
        group: group name
        mode: mode to set

    Returns:
        list[RequirementState]: list of requirement states
    """
    states: list[RequirementState] = []

    sudo = "sudo " if not is_root() else ""
    user_id = pwd.getpwnam(user).pw_uid
    group_id = grp.getgrnam(group).gr_gid
    mode_str = "0" + oct(mode)[2:]  # Remove the 0o prefix, keep "0" instead

    for d in dirs:
        exists = d.exists()

        if not exists:
            if not fix:
                states.append(RequirementState(Status.FAIL, f"Directory {d} does not exist.", fixable=True))
            else:
                ctx.run(f"{sudo}install -d -m 0{mode_str} -g {group} -o {user} {d}")
                states.append(RequirementState(Status.OK, f"Created missing KMT directory: {d}"))

        perms = d.stat().st_mode
        if perms & 0o777 != (mode & 0o777):  # Check only the permission bits
            if not fix:
                states.append(
                    RequirementState(
                        Status.FAIL,
                        f"Directory {d} has incorrect permissions {oct(perms)}, want {mode_str}.",
                        fixable=True,
                    )
                )
            else:
                ctx.run(f"{sudo}chmod {mode_str} {d}")
                states.append(RequirementState(Status.OK, f"Fixed permissions for KMT directory: {d}"))

        owner_id = d.stat().st_uid
        actual_group_id = d.stat().st_gid
        if owner_id != user_id or actual_group_id != group_id:
            if not fix:
                states.append(
                    RequirementState(
                        Status.FAIL,
                        f"Directory {d} has incorrect owner or group (uid={owner_id}, gid={actual_group_id}; wanted uid={user_id}, gid={group_id}).",
                        fixable=True,
                    )
                )
            else:
                ctx.run(f"{sudo}chown -R {user}:{group} {d}")
                states.append(RequirementState(Status.OK, f"Fixed owner and group for KMT directory: {d}"))

    return states


class _BasePackageManager:
    """
    Base class for package checkers.

    Args:
        ctx: invoke context
    """

    def __init__(self, ctx: Context):
        self.ctx = ctx

    def _package_exists(self, package: str) -> bool:
        raise NotImplementedError("Subclasses must implement this method")

    def _install_packages(self, packages: list[str]) -> None:
        raise NotImplementedError("Subclasses must implement this method")

    def check(self, packages: list[str], fix: bool) -> RequirementState:
        missing_packages: list[str] = []
        for package in packages:
            if not self._package_exists(package):
                missing_packages.append(package)

        if len(missing_packages) == 0:
            return RequirementState(Status.OK, "All packages are installed.")

        if not fix:
            return RequirementState(Status.FAIL, f"Missing packages: {', '.join(missing_packages)}", fixable=True)

        try:
            self._install_packages(missing_packages)
        except Exception as e:
            return RequirementState(Status.FAIL, f"Failed to install packages: {e}")

        return RequirementState(Status.OK, "Packages installed.")


class UbuntuPackageManager(_BasePackageManager):
    def _package_exists(self, package: str) -> bool:
        return cast(Result, self.ctx.run(f"dpkg -s {package}", warn=True)).ok

    def _install_packages(self, packages: list[str]) -> None:
        sudo = "sudo " if not is_root() else ""
        self.ctx.run(f"{sudo}apt update")
        self.ctx.run(f"{sudo}apt install -y {' '.join(packages)}")


class MacosPackageManager(_BasePackageManager):
    def _package_exists(self, package: str) -> bool:
        return cast(Result, self.ctx.run(f"brew list {package}", warn=True)).ok

    def _install_packages(self, packages: list[str]) -> None:
        self.ctx.run("brew update")
        self.ctx.run(f"brew install {' '.join(packages)}")


class UbuntuSnapPackageManager(_BasePackageManager):
    def __init__(self, ctx: Context, classic: bool = False):
        super().__init__(ctx)
        self.classic = classic

    def _package_exists(self, package: str) -> bool:
        return cast(Result, self.ctx.run(f"snap list {package}", warn=True)).ok

    def _install_packages(self, packages: list[str]) -> None:
        sudo = "sudo " if not is_root() else ""
        cmd = f"{sudo}snap install {' '.join(packages)}"
        if self.classic:
            cmd += " --classic"
        self.ctx.run(cmd)


class PipPackageManager(_BasePackageManager):
    @property
    def pip_command(self) -> str:
        # quote the executable to avoid issues with spaces in the path
        # we use the python executable currently running the script, so that we
        # always install in the same environment as the script
        return f"\"{sys.executable}\" -m pip"

    def _package_exists(self, package: str) -> bool:
        if "==" in package:
            version = package.split("==")[1]
            package = package.split("==")[0]
        else:
            version = None

        res = self.ctx.run(f"{self.pip_command} list --format=columns | grep {package}", warn=True)
        if res is None or not res.ok:
            return False

        if version is None:
            return True

        parts = res.stdout.strip().splitlines()[0].split()
        if len(parts) < 2:
            return False

        return parts[1] == version

    def _install_packages(self, packages: list[str]) -> None:
        self.ctx.run(f"{self.pip_command} install {' '.join(packages)}")


def check_user_in_group(ctx: Context, group: str, fix=False) -> RequirementState:
    res = ctx.run(
        f"cat /proc/$$/status | grep '^Groups:' | grep $(cat /etc/group | grep '{group}:' | cut -d ':' -f 3)", warn=True
    )
    if res is not None and res.ok:
        return RequirementState(Status.OK, f"User is in group {group}.")

    if not fix:
        return RequirementState(Status.FAIL, f"User is not in group {group}.", fixable=fix)

    sudo = "sudo " if not is_root() else ""
    ctx.run(f"{sudo}usermod -aG {group} {getpass.getuser()}")

    return RequirementState(Status.WARN, f"User added to group {group}, you will need to log out and back in to use it. Subsequent requirements might not work correctly.")
