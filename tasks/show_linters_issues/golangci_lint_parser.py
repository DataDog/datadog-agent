"""
This module defines a parser for golangci-lint output.
"""

import re
import os
from collections import defaultdict
from typing import Dict
from ..libs.pipeline_notifications import read_owners

# Example lint message
# "pointer.go:6:1: package-comments: should have a package comment (revive)"
LINT_PATTERN = re.compile("^([^:]+):([0-9]+):([0-9]+): (([^:]+): )?(.+) \\((.+)\\)$")

# Example module message
# "Linters for module /Users/pierre.gimalac/go/src/github.com/DataDog/datadog-agent/pkg/remoteconfig/state failed (base flavor)"
MODULE_PATTERN = re.compile("^.*Linters for module ([^ ]+) failed \\((.+)\\).*$")

CODEOWNERS_FILE_PATH = ".github/CODEOWNERS"

def is_team_owner(file: str, team: str) -> bool:
    codeowners = read_owners(CODEOWNERS_FILE_PATH)
    team = team.lower()
    file_owners = codeowners.of(file)
    file_owners = [(x[0], x[1].lower()) for x in file_owners]
    return ('TEAM', team) in file_owners


def parse_file(golangci_lint_output: str):
    """
    Parses the output of the golangci-lint run.
    Returns a Dict(linter: List(base_path, row, col, lint, descr)).
    """
    lints = []
    current_module = None
    for line in golangci_lint_output.split("\n"):
        line = line.strip()

        match = re.match(MODULE_PATTERN, line)
        if match:
            module_path, flavor = match.groups()
            current_module = module_path
            continue

        if current_module is None:
            continue

        match = re.match(LINT_PATTERN, line)
        if match is None:
            continue

        file_path, row, col, lint, _, descr, linter = match.groups()
        full_path = os.path.normpath(os.path.join(current_module, file_path))
        base_path = full_path.removeprefix(os.getcwd()).removeprefix('/')
        lints.append((base_path, row, col, lint, descr, linter))

    lints_per_linter = defaultdict(list)
    for base_path, row, col, lint, descr, linter in lints:
        lints_per_linter[linter].append((base_path, row, col, lint, descr))

    return lints_per_linter


def filter_lints(lints_per_linter, filter_team: str=None, filter_linters: str=None):
    """
    Keeps only the lints from a specific team and specific linters.

        Parameters:
            filter_team (str): Keep only the lints from the files owned by the filter_team. None will keep lints from all teams.
            filter_linters (str): Comma-separated linters to keep. None will keep lints from all linters.
    """
    list_filter_linters = filter_linters.split(',') if filter_linters else []
    filtered_lints = defaultdict(set)
    for linter in lints_per_linter:
        # If either we didn't set a filter or the linter is in the filter list
        if not filter_linters or linter in list_filter_linters:
            if not filter_team:
                filtered_lints[linter] = lints_per_linter[linter]
            else:
                # Filter only the lints owned by the filter_team
                for lint in lints_per_linter[linter]:
                    if is_team_owner(lint[0], filter_team):
                        filtered_lints[linter].add(lint)
    return filtered_lints


def display_result(filtered_lints):
    """
    Displays results
    """
    if not filtered_lints:
        return "No linter error !"
    output = ""
    for linter in filtered_lints:
        output+= f"[{linter}]\n"
        for lint in filtered_lints[linter]:
            output += f"\n{lint[0]}:{lint[1]}:{lint[2]} {lint[3]}{lint[4]}"
        output += "\n"
    return output

def merge_results(results_per_os_x_arch: Dict[str, str]):
    """
    Merge golangci-lint output
    """
    merged_lints_per_linter = defaultdict(set)
    for _, result in results_per_os_x_arch.items():
        lints_per_linter = parse_file(result)
        for linter, lints in lints_per_linter.items():
            merged_lints_per_linter[linter].update(lints)
    return merged_lints_per_linter
