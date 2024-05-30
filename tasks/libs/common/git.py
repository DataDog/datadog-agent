def get_staged_files(ctx, commit="HEAD") -> list[str]:
    """
    Get the list of staged (to be committed) files in the repository compared to the `commit` commit.
    """
    return ctx.run(f"git diff --name-only --staged {commit}", hide=True).stdout.strip().splitlines()


def get_modified_files(ctx) -> list[str]:
    last_main_commit = ctx.run("git merge-base HEAD origin/main", hide=True).stdout
    return ctx.run(f"git diff --name-only --no-renames {last_main_commit}", hide=True).stdout.splitlines()
