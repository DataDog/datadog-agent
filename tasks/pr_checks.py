import os

from invoke import Exit, task

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.utils import DEFAULT_BRANCH, GITHUB_REPO_NAME


def is_pr_context(branch, pr_url, test_name):
    if branch == DEFAULT_BRANCH:
        print(f"Running on {DEFAULT_BRANCH}, skipping check for {test_name}.")
        return False
    if not pr_url:
        print(f"PR not found, skipping check for {test_name}.")
        return False
    return True


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
                    ", see https://github.com/DataDog/datadog-agent/blob/main/docs/dev/contributing.md#reno"
                    ", or apply the label 'changelog/no-changelog' to the PR."
                )
                raise Exit(code=1)
        else:
            print("'changelog/no-changelog' label found on the PR: skipping linting")

    ctx.run("reno lint")
