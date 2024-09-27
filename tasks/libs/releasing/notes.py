from datetime import date

from invoke import Failure

from tasks.libs.common.constants import DEFAULT_BRANCH, GITHUB_REPO_NAME
from tasks.libs.releasing.version import current_version


def _add_prelude(ctx, version):
    res = ctx.run(f"reno new prelude-release-{version}")
    new_releasenote = res.stdout.split(' ')[-1].strip()  # get the new releasenote file path

    with open(new_releasenote, "w") as f:
        f.write(
            f"""prelude:
    |
    Release on: {date.today()}

    - Please refer to the `{version} tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-{version.replace('.', '')}>`_ for the list of changes on the Core Checks
"""
        )

    ctx.run(f"git add {new_releasenote}")
    print("\nIf not run as part of finish task, commit this with:")
    print(f"git commit -m \"Add prelude for {version} release\"")


def _add_dca_prelude(ctx, agent7_version, agent6_version=""):
    """
    Release of the Cluster Agent should be pinned to a version of the Agent.
    """
    res = ctx.run(f"reno --rel-notes-dir releasenotes-dca new prelude-release-{agent7_version}")
    new_releasenote = res.stdout.split(' ')[-1].strip()  # get the new releasenote file path

    if agent6_version != "":
        agent6_version = (
            f"--{agent6_version.replace('.', '')}"  # generate the right hyperlink to the agent's changelog.
        )

    with open(new_releasenote, "w") as f:
        f.write(
            f"""prelude:
    |
    Released on: {date.today()}
    Pinned to datadog-agent v{agent7_version}: `CHANGELOG <https://github.com/{GITHUB_REPO_NAME}/blob/{DEFAULT_BRANCH}/CHANGELOG.rst#{agent7_version.replace('.', '')}{agent6_version}>`_."""
        )

    ctx.run(f"git add {new_releasenote}")
    print("\nIf not run as part of finish task, commit this with:")
    print(f"git commit -m \"Add prelude for {agent7_version} release\"")


def update_changelog_generic(ctx, new_version, changelog_dir, changelog_file):
    if new_version is None:
        latest_version = current_version(ctx, 7)
        ctx.run(f"reno -q --rel-notes-dir {changelog_dir} report --ignore-cache --earliest-version {latest_version}")
        return
    new_version_int = list(map(int, new_version.split(".")))

    # removing releasenotes from bugfix on the old minor.
    branching_point = f"{new_version_int[0]}.{new_version_int[1]}.0-devel"
    previous_minor = f"{new_version_int[0]}.{new_version_int[1] - 1}"
    if previous_minor == "7.15":
        previous_minor = "6.15"  # 7.15 is the first release in the 7.x series
    log_result = ctx.run(
        f"git log {branching_point}...remotes/origin/{previous_minor}.x --name-only --oneline | grep {changelog_dir}/notes/ || true"
    )
    log_result = log_result.stdout.replace('\n', ' ').strip()
    if len(log_result) > 0:
        ctx.run(f"git rm --ignore-unmatch {log_result}")

    # generate the new changelog
    ctx.run(
        f"reno --rel-notes-dir {changelog_dir} report --ignore-cache --earliest-version {branching_point} --version {new_version} --no-show-source > /tmp/new_changelog.rst"
    )

    ctx.run(f"git checkout HEAD -- {changelog_dir}")

    # mac's `sed` has a different syntax for the "-i" paramter
    # GNU sed has a `--version` parameter while BSD sed does not, using that to do proper detection.
    try:
        ctx.run("sed --version", hide='both')
        sed_i_arg = "-i"
    except Failure:
        sed_i_arg = "-i ''"
    # remove the old header from the existing changelog
    ctx.run(f"sed {sed_i_arg} -e '1,4d' {changelog_file}")

    # merging to <changelog_file>
    ctx.run(f"cat {changelog_file} >> /tmp/new_changelog.rst && mv /tmp/new_changelog.rst {changelog_file}")

    # commit new CHANGELOG
    ctx.run(f"git add {changelog_file}")

    print("\nCommit this with:")
    print(f"git commit -m \"Update {changelog_file} for {new_version}\"")
