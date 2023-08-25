"""
Invoke tasks to fix the linter
"""

from invoke import task
from invoke.exceptions import Exit
from ..libs.pipeline_notifications import GITHUB_SLACK_MAP
from .golangci_lint_parser import filter_lints, display_result, parse_file

FIRST_COMMIT_HASH = "52a313fe7f5e8e16d487bc5dc770038bc234608b"
CI_TESTED_OS_AND_ARCH = {"linux": ["amd64", "arm64"], "windows": ["amd64"]}

def check_if_team_exists(team: str):
    """
    Check if an input team exists in the GITHUB_SLACK_MAP. Exits the code if it doesn't.
    """
    if team:
        team = team.lower()
        if not team in GITHUB_SLACK_MAP:
            raise Exit(f"=> Team '{team}' does not exist.\n=> Your team should be in {[t for t in GITHUB_SLACK_MAP]}", code=2)
    else:
        print("[WARNING] No team entered. Displaying linters errors for all teams.\n")

# run on both win_x64 and linux (arm & amd) using env = {"GOOS": "linux", "GOARCH": "amd64"}

@task
def need_fixing_linters(ctx, filter_team: str=None, from_commit_hash: str=FIRST_COMMIT_HASH, filter_linters: str="revive,gosimple", show_output=False, tested_os_and_arch=CI_TESTED_OS_AND_ARCH):
    """
    This function allows you to display the list of files that need fixing for a specific team and for specific linters.

        Example: inv -e need-fixing-linters --filter-team "@DataDog/agent-platform" --filter-linters "revive"

        Parameters:
            team (str): keep only the files owned by a team. These are Github team names from the GITHUB_SLACK_MAP variable.
            from_commit_hash (str): the linter will run on all commit after this hash. Set on the first commit on the repo by default.
            filter_linters (str): comma separated string of the linters you want to keep in the output. By default no filter applied.
            show_output (bool): show output of the raw linter run.
            [TODO] tested_os_and_arch (dict): dict of the OS and their arch on which the linter will run

    """
    check_if_team_exists(filter_team)
    golangci_lint_kwargs=f'"--new-from-rev {from_commit_hash} --print-issued-lines=false"'
    command = f"inv -e lint-go --golangci-lint-kwargs {golangci_lint_kwargs}"

    # TODO: One run per OSxArch
    # TODO: One run per flavor
    for tested_os, tested_arch in tested_os_and_arch.items():
        env = {"GOOS": tested_os, "GOARCH": tested_arch}
    env = {}
    result = ctx.run(command, env=env, warn=True, hide=not show_output)
    lints_filtered_by_team = filter_lints(parse_file(result.stdout), filter_team, filter_linters)
    display = display_result(lints_filtered_by_team)
    if filter_team:
        print(f"Results of running '{filter_linters}' linters on {filter_team} team owned files:\n")
    print(display)
