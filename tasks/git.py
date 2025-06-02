import re

from invoke import Context, task
from invoke.exceptions import Exit

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.color import color_message
from tasks.libs.common.git import create_tree, get_current_branch, get_default_branch


@task
def check_protected_branch(ctx: Context) -> None:
    """Test if we are trying to commit or push to a protected branch."""
    local_branch = get_current_branch(ctx)

    if local_branch == get_default_branch():
        print(
            color_message(
                f"You're about to commit or push to {get_default_branch()}, are you sure this is what you want?", "red"
            )
        )
        raise Exit(code=1)

    if re.fullmatch(r'^[0-9]+\.[0-9]+\.x$', local_branch):
        print(
            color_message(
                "You're about to commit or push to a release branch, are you sure this is what you want?", "red"
            )
        )
        raise Exit(code=1)


@task
def push_signed_commits(ctx: Context, branch: str, commit_message: str, source_branch: str | None = None) -> None:
    """Create a tree from local stage changes, commit and push using API to get signed commits from bots.

    Args:
        ctx: Invoke context
        update_branch: The branch to push to
        commit_message: The commit message to use
    """
    print("Creating signed commits using Github API")
    github = GithubAPI()
    tree = create_tree(ctx, source_branch or get_default_branch())
    github.commit_and_push_signed(branch, commit_message, tree)
