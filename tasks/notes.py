from invoke import Failure, task
from invoke.exceptions import Exit

from tasks.libs.ciproviders.github_api import create_release_pr
from tasks.libs.common.color import color_message
from tasks.libs.common.git import get_current_branch, try_git_command
from tasks.libs.common.worktree import agent_context
from tasks.libs.releasing.notes import _add_dca_prelude, _add_prelude, update_changelog_generic
from tasks.libs.releasing.version import deduce_version


@task
def add_prelude(ctx, release_branch):
    version = deduce_version(ctx, release_branch, next_version=False)

    with agent_context(ctx, release_branch):
        _add_prelude(ctx, version)


@task
def add_dca_prelude(ctx, release_branch):
    """
    Release of the Cluster Agent should be pinned to a version of the Agent.
    """
    version = deduce_version(ctx, release_branch, next_version=False)

    with agent_context(ctx, release_branch):
        _add_dca_prelude(ctx, version)


@task
def update_changelog(ctx, release_branch, target="all", upstream="origin"):
    """
    Quick task to generate the new CHANGELOG using reno when releasing a minor
    version (linux/macOS only).
    By default generates Agent and Cluster Agent changelogs.
    Use target == "agent" or target == "cluster-agent" to only generate one or the other.
    If new_version is omitted, a changelog since last tag on the current branch
    will be generated.
    """

    with agent_context(ctx, release_branch):
        new_version = deduce_version(ctx, release_branch, next_version=False)
        new_version_int = list(map(int, new_version.split(".")))
        if len(new_version_int) != 3:
            print(f"Error: invalid version: {new_version_int}")
            raise Exit(code=1)

        # Step 1 - generate the changelogs

        generate_agent = target in ["all", "agent"]
        generate_cluster_agent = target in ["all", "cluster-agent"]

        # let's avoid losing uncommitted change with 'git reset --hard'
        try:
            ctx.run("git diff --exit-code HEAD", hide="both")
        except Failure:
            print("Error: You have uncommitted change, please commit or stash before using update_changelog")
            return

        # let's check that the tag for the new version is present
        try:
            ctx.run(f"git tag --list | grep {new_version}")
        except Failure:
            print(f"Missing '{new_version}' git tag: mandatory to use 'reno'")
            raise

        if generate_agent:
            update_changelog_generic(ctx, new_version, "releasenotes", "CHANGELOG.rst")
        if generate_cluster_agent:
            update_changelog_generic(ctx, new_version, "releasenotes-dca", "CHANGELOG-DCA.rst")

        # Step 2 - commit changes

        update_branch = f"changelog-update-{new_version}"
        base_branch = get_current_branch(ctx)

        print(color_message(f"Branching out to {update_branch}", "bold"))
        ctx.run(f"git checkout -b {update_branch}")

        print(color_message("Committing CHANGELOG.rst and CHANGELOG-DCA.rst", "bold"))
        print(
            color_message(
                "If commit signing is enabled, you will have to make sure the commit gets properly signed.", "bold"
            )
        )
        ctx.run("git add CHANGELOG.rst CHANGELOG-DCA.rst")

        commit_message = f"'Changelog updates for {new_version} release'"

        ok = try_git_command(ctx, f"git commit -m {commit_message}")
        if not ok:
            raise Exit(
                color_message(
                    f"Could not create commit. Please commit manually with:\ngit commit -m {commit_message}\n, push the {update_branch} branch and then open a PR.",
                    "red",
                ),
                code=1,
            )

        # Step 3 - Push and create PR

        print(color_message("Pushing new branch to the upstream repository", "bold"))
        res = ctx.run(f"git push --set-upstream {upstream} {update_branch}", warn=True)
        if res.exited is None or res.exited > 0:
            raise Exit(
                color_message(
                    f"Could not push branch {update_branch} to the upstream '{upstream}'. Please push it manually and then open a PR.",
                    "red",
                ),
                code=1,
            )

        create_release_pr(
            f"Changelog update for {new_version} release",
            base_branch,
            update_branch,
            new_version,
            changelog_pr=True,
            milestone=str(new_version),
        )
