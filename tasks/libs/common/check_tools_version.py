from __future__ import annotations

import json
import sys

from invoke import Context, Exit

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.utils import gitlab_section

# VPATH as in version path
GO_VPATH = ".go-version"
GOLANGCI_LINT_VPATH = "internal/tools/go.mod"


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


def expected_golangci_lint_repo_v(ctx: Context) -> str:
    """
    Returns the installed golangci-lint version by parsing the internal/tools/go.mod file.
    """
    mod_name = "github.com/golangci/golangci-lint"
    go_mod_json = json.loads(ctx.run(f"go mod edit -json {GOLANGCI_LINT_VPATH}", hide=True).stdout)
    for mod in go_mod_json['Require']:
        if mod['Path'] == mod_name:
            return mod['Version']
    return ""


def custom_golangci_v(v: str) -> str:
    """
    Returns the golangci-lint version with the custom suffix.
    """
    return f"{v}-custom-gcl"


def current_golangci_lint_v(ctx: Context, debug: bool = False) -> str:
    """
    Returns the current user golangci-lint version by running golangci-lint --version
    """
    debug_flag = "--debug" if debug else ""
    cmd = f"golangci-lint version {debug_flag}"
    version_output = ctx.run(cmd, hide=True).stdout
    return version_output if debug else version_output.split(' ')[3]


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
        'golangci-lint': {
            'current_v': current_golangci_lint_v(ctx),
            'expected_v': custom_golangci_v(expected_golangci_lint_repo_v(ctx)),
            'debug': '' if not debug else current_golangci_lint_v(ctx, debug=debug),
            'exit_on_error': True,
            'error_msg': "Error: The golanci-lint version you are using is not the correct one. Please run inv -e setup to install the correct version.",
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
                        f"Expecting {tool} '{expected_v}' but you have {tool} '{current_v}'. Please run inv -e install-tools to fix this as you might encounter issues using the tooling.",
                        Color.RED,
                    ),
                    file=sys.stderr,
                )
                should_exit = should_exit or tools_versions[tool]['exit_on_error']
    if should_exit:
        raise Exit(code=1)
    return True
