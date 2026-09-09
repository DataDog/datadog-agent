from __future__ import annotations

import sys

from invoke import Context, Exit

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.utils import gitlab_section

# VPATH as in version path
GO_VPATH = ".go-version"


def expected_go_repo_v() -> str:
    """
    Returns the repository go version by reading the .go-version file.
    """
    with open(GO_VPATH, encoding='utf-8') as f:
        return f.read().strip()


def current_go_v(ctx: Context) -> str:
    """
    Returns the current user go version by running go version
    """
    cmd = "go version"
    return ctx.run(cmd, hide=True).stdout.split(' ')[2][2:]


def check_tools_version(ctx: Context, tools_list: list[str], debug: bool = False) -> bool:
    """
    Check that each installed tool in tools_list is the version expected for the repo.
    """
    should_exit = False
    tools_versions = {
        'go': {
            'current_v': current_go_v(ctx),
            'expected_v': expected_go_repo_v(),
            'debug': '' if not debug else current_go_v(ctx),
            'exit_on_error': False,
            'error_msg': "Warning: If you have linter errors it might be due to version mismatches.",
        },
    }
    for tool in tools_list:
        if debug:
            with gitlab_section(f"{tool} debug info", collapsed=True):
                print(tools_versions[tool]['debug'])
        if tool not in tools_versions:
            print(
                f"Warning: Couldn't check '{tool}' expected version. Supported tools: {list(tools_versions.keys())}",
                file=sys.stderr,
            )
        else:
            current_v, expected_v = tools_versions[tool]['current_v'], tools_versions[tool]['expected_v']
            if current_v != expected_v:
                print(
                    color_message(
                        f"Expecting {tool} '{expected_v}' but you have {tool} '{current_v}'. Please run dda inv -e install-tools to fix this as you might encounter issues using the tooling.",
                        Color.RED,
                    ),
                    file=sys.stderr,
                )
                should_exit = should_exit or tools_versions[tool]['exit_on_error']
    if should_exit:
        raise Exit(code=1)
    return True
