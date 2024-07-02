from __future__ import annotations

import json
import sys

from invoke import Context

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


def current_golangci_lint_v(ctx: Context) -> str:
    """
    Returns the current user golangci-lint version by running golangci-lint --version
    """
    cmd = "golangci-lint --version"
    return ctx.run(cmd, hide=True).stdout.split(' ')[3]


def check_tools_version(ctx: Context, tools_list: list[str]) -> bool:
    """
    Check that each installed tool in tools_list is the version expected for the repo.
    """
    is_expected_versions = True
    tools_versions = {
        'go': {'current_v': current_go_v(ctx), 'expected_v': expected_go_repo_v()},
        'golangci-lint': {'current_v': current_golangci_lint_v(ctx), 'expected_v': expected_golangci_lint_repo_v(ctx)},
    }
    for tool in tools_list:
        if tool not in tools_versions:
            print(
                f"Warning: Couldn't check '{tool}' expected version. Supported tools: {list(tools_versions.keys())}",
                file=sys.stderr,
            )
        else:
            current_v, expected_v = tools_versions[tool]['current_v'], tools_versions[tool]['expected_v']
            if current_v != expected_v:
                is_expected_versions = False
                print(
                    f"Warning: Expecting {tool} '{expected_v}' but you have {tool} '{current_v}'. Please fix this as you might encounter issues using the tooling.",
                    file=sys.stderr,
                )
    return is_expected_versions
