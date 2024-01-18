import os

from invoke import Exit, task

from .libs.common.github_api import GithubAPI
from .libs.common.utils import DEFAULT_BRANCH, GITHUB_REPO_NAME


def is_pr_context(branch, pr_url, test_name):
    if branch == DEFAULT_BRANCH:
        print(f"Running on {DEFAULT_BRANCH}, skipping check for {test_name}.")
        return False
    if not pr_url:
        print(f"PR not found, skipping check for {test_name}.")
        return False
    return True


@task
def lint_teamassignment(_):
    """
    Make sure PRs are assigned a team label
    """
    branch = os.environ.get("BRANCH_NAME")
    pr_url = os.environ.get("PR_ID")

    run_check = is_pr_context(branch, pr_url, "team assignment")
    if run_check:
        github = GithubAPI(repository=GITHUB_REPO_NAME, public_repo=True)
        pr_id = pr_url.rsplit('/')[-1]

        need_check, team_labels = github.get_team_assignment_labels(pr_id)
        if need_check:
            if len(team_labels) > 0:
                print(f"Team Assignment: {team_labels}")
            else:
                print(f"PR {pr_url} requires at least one non-triage team assignment label (label starting by 'team/')")
                raise Exit(code=1)
        else:
            print("A label to skip QA is set -- no need for team assignment")


@task
def lint_skip_qa(_):
    """
    Ensure that when qa/skip-qa is used, we have one of [qa/done , qa/no-code-change]. Error if not valid.
    """
    branch = os.environ.get("BRANCH_NAME")
    pr_url = os.environ.get("PR_ID")

    run_check = is_pr_context(branch, pr_url, "skip-qa")
    if run_check:
        github = GithubAPI(repository=GITHUB_REPO_NAME, public_repo=True)
        pr_id = pr_url.rsplit('/')[-1]
        if not github.is_qa_skip_ok(pr_id):
            print(
                f"PR {pr_url} request to skip QA without justification. Requires an additional `qa/done` or `qa/no-code-change`."
            )
            raise Exit(code=1)
        return


@task
def lint_milestone(_):
    """
    Make sure PRs are assigned a milestone
    """
    branch = os.environ.get("BRANCH_NAME")
    pr_url = os.environ.get("PR_ID")

    run_check = is_pr_context(branch, pr_url, "milestone")
    if run_check:
        github = GithubAPI(repository=GITHUB_REPO_NAME, public_repo=True)
        pr_id = pr_url.rsplit('/')[-1]

        milestone = github.get_pr_milestone(pr_id)
        if milestone and milestone != "Triage":
            print(f"Milestone: {milestone}")
        else:
            print(f"PR {pr_url} requires a non-Triage milestone.")
            raise Exit(code=1)


@task
def lint_releasenote(ctx):
    """
    Lint release notes with Reno
    """
    branch = os.environ.get("CIRCLE_BRANCH")
    pr_url = os.environ.get("CIRCLE_PULL_REQUEST")

    run_check = is_pr_context(branch, pr_url, "team assignment")
    if run_check:
        github = GithubAPI(repository=GITHUB_REPO_NAME, public_repo=True)
        pr_id = pr_url.rsplit('/')[-1]
        if github.is_release_note_needed(pr_id):
            if not github.contains_release_note(pr_id):
                print(
                    "Error: No releasenote was found for this PR. Please add one using 'reno'"
                    ", or apply the label 'changelog/no-changelog' to the PR."
                )
                raise Exit(code=1)
        else:
            print("'changelog/no-changelog' label found on the PR: skipping linting")

    ctx.run("reno lint")
