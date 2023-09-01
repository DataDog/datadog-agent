"""
Invoke tasks to fix the linter
"""

from invoke import task
from invoke.exceptions import Exit

from ..libs.pipeline_notifications import GITHUB_SLACK_MAP
from .golangci_lint_parser import display_result, filter_lints, merge_results

FIRST_COMMIT_HASH = "52a313fe7f5e8e16d487bc5dc770038bc234608b"
# See https://go.dev/doc/install/source#environment for all available combinations of GOOS x GOARCH.
CI_TESTED_OS_AND_ARCH = ["linux,arm64", "linux,amd64", "windows,amd64", "darwin,amd64"]


def check_if_team_exists(team: str):
    """
    Check if an input team exists in the GITHUB_SLACK_MAP. Exits the code if it doesn't.
    """
    if team:
        if team not in GITHUB_SLACK_MAP:
            raise Exit(f"=> Team '{team}' does not exist.\n=> Your team should be in {GITHUB_SLACK_MAP}", code=2)
    else:
        print("[WARNING] No team entered. Displaying linters errors for all teams.\n")


@task(iterable=['tested_os_and_arch'])
def show_linters_issues(
    ctx,
    filter_team: str = None,
    from_commit_hash: str = FIRST_COMMIT_HASH,
    filter_linters: str = "revive",
    show_output=False,
    platforms=None,
):
    """
    This function displays the list of files that need fixing for a specific team and for specific linters.

        Example: inv show-linters-issues --filter-team "@DataDog/agent-platform" --filter-linters "revive" --platforms "linux,amd64" --platforms "linux,arm64"

        Parameters:
            team (str): keep only the files owned by a team. These are Github team names from the GITHUB_SLACK_MAP variable.
            from_commit_hash (str): the linters will run on all commit after this hash. Set on the first commit on the repo by default.
            filter_linters (str): comma-separated string of the linters you want to keep in the output. By default no filter applied.
            show_output (bool): show output of the raw linter run.
            platforms (list): list of comma-separated OS,arch on which the linter will run.

    """
    if filter_team:
        filter_team = filter_team.lower()
    check_if_team_exists(filter_team)
    golangci_lint_kwargs = f'"--new-from-rev {from_commit_hash} --print-issued-lines=false"'
    command = f"inv lint-go --golangci-lint-kwargs {golangci_lint_kwargs}"

    # Run the linters for every OS x Arch and merge the results
    results_per_os_x_arch = dict()
    platforms = platforms if platforms else CI_TESTED_OS_AND_ARCH
    platforms = [p.split(',') for p in platforms]
    for tested_os, tested_arch in platforms:
        env = {"GOOS": tested_os, "GOARCH": tested_arch}
        print(f"Running linters for {tested_os}_{tested_arch}...")
        results_per_os_x_arch[f"{tested_os}_{tested_arch}"] = ctx.run(
            command, env=env, warn=True, hide=not show_output
        ).stdout
    merged_results = merge_results(results_per_os_x_arch)

    # Filter the results by filtering with filter_team and filter_linters.
    lints_filtered_by_team = filter_lints(merged_results, filter_team, filter_linters)
    display = display_result(lints_filtered_by_team)
    if filter_team:
        print(f"Results of running '{filter_linters}' linters on {filter_team} team owned files:\n")
    print(display)
