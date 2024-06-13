import sys
from datetime import date

from invoke import Failure, task
from invoke.exceptions import Exit

from tasks.libs.ciproviders.github_api import create_release_pr
from tasks.libs.common.color import color_message
from tasks.libs.common.git import get_current_branch, try_git_command
from tasks.libs.releasing.notes import _add_dca_prelude, _add_prelude, update_changelog_generic


@task
def add_prelude(ctx, version):
    _add_prelude(ctx, version)


@task
def add_dca_prelude(ctx, agent7_version, agent6_version=""):
    """
    Release of the Cluster Agent should be pinned to a version of the Agent.
    """
    _add_dca_prelude(ctx, agent7_version, agent6_version)


@task
def add_installscript_prelude(ctx, version):
    res = ctx.run(f"reno --rel-notes-dir releasenotes-installscript new prelude-release-{version}")
    new_releasenote = res.stdout.split(' ')[-1].strip()  # get the new releasenote file path

    with open(new_releasenote, "w") as f:
        f.write(
            f"""prelude:
    |
    Released on: {date.today()}"""
        )

    ctx.run(f"git add {new_releasenote}")
    print("\nCommit this with:")
    print(f"git commit -m \"Add prelude for {version} release\"")


@task
def update_changelog(ctx, new_version=None, target="all", upstream="origin"):
    """
    Quick task to generate the new CHANGELOG using reno when releasing a minor
    version (linux/macOS only).
    By default generates Agent and Cluster Agent changelogs.
    Use target == "agent" or target == "cluster-agent" to only generate one or the other.
    If new_version is omitted, a changelog since last tag on the current branch
    will be generated.
    """

    # Step 1 - generate the changelogs

    generate_agent = target in ["all", "agent"]
    generate_cluster_agent = target in ["all", "cluster-agent"]

    if new_version is not None:
        new_version_int = list(map(int, new_version.split(".")))
        if len(new_version_int) != 3:
            print(f"Error: invalid version: {new_version_int}")
            raise Exit(1)

        # let's avoid losing uncommitted change with 'git reset --hard'
        try:
            ctx.run("git diff --exit-code HEAD", hide="both")
        except Failure:
            print("Error: You have uncommitted change, please commit or stash before using update_changelog")
            return

        # make sure we are up to date
        ctx.run("git fetch")

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
        f"Changelog update for {new_version} release", base_branch, update_branch, new_version, changelog_pr=True
    )


@task
def update_installscript_changelog(ctx, new_version):
    """
    Quick task to generate the new CHANGELOG-INSTALLSCRIPT using reno when releasing a minor
    version (linux/macOS only).
    """
    new_version_int = list(map(int, new_version.split(".")))

    if len(new_version_int) != 3:
        print(f"Error: invalid version: {new_version_int}")
        raise Exit(1)

    # let's avoid losing uncommitted change with 'git reset --hard'
    try:
        ctx.run("git diff --exit-code HEAD", hide="both")
    except Failure:
        print("Error: You have uncommitted changes, please commit or stash before using update-installscript-changelog")
        return

    # make sure we are up to date
    ctx.run("git fetch")

    # let's check that the tag for the new version is present (needed by reno)
    try:
        ctx.run(f"git tag --list | grep installscript-{new_version}")
    except Failure:
        print(f"Missing 'installscript-{new_version}' git tag: mandatory to use 'reno'")
        raise

    # generate the new changelog
    ctx.run(
        f"reno --rel-notes-dir releasenotes-installscript report             --ignore-cache             --version installscript-{new_version}             --no-show-source > /tmp/new_changelog-installscript.rst"
    )

    # reseting git
    ctx.run("git reset --hard HEAD")

    # mac's `sed` has a different syntax for the "-i" paramter
    sed_i_arg = "-i"
    if sys.platform == 'darwin':
        sed_i_arg = "-i ''"
    # remove the old header from the existing changelog
    ctx.run(f"sed {sed_i_arg} -e '1,4d' CHANGELOG-INSTALLSCRIPT.rst")

    if sys.platform != 'darwin':
        # sed on darwin doesn't support `-z`. On mac, you will need to manually update the following.
        ctx.run(
            "sed -z {0} -e 's/installscript-{1}\\n===={2}/{1}\\n{2}/' /tmp/new_changelog-installscript.rst".format(  # noqa: FS002
                sed_i_arg, new_version, '=' * len(new_version)
            )
        )

    # merging to CHANGELOG-INSTALLSCRIPT.rst
    ctx.run(
        "cat CHANGELOG-INSTALLSCRIPT.rst >> /tmp/new_changelog-installscript.rst && mv /tmp/new_changelog-installscript.rst CHANGELOG-INSTALLSCRIPT.rst"
    )

    # commit new CHANGELOG-INSTALLSCRIPT
    ctx.run("git add CHANGELOG-INSTALLSCRIPT.rst")

    print("\nCommit this with:")
    print(f"git commit -m \"[INSTALLSCRIPT] Update CHANGELOG-INSTALLSCRIPT for {new_version}\"")
