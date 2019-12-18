"""
Release helper tasks
"""
from __future__ import print_function
import re
import sys
from datetime import date

from invoke import task, Failure
from invoke.exceptions import Exit, UnexpectedExit


@task
def add_prelude(ctx, version):
    res = ctx.run("reno new prelude-release-{0}".format(version))
    new_releasenote = res.stdout.split(' ')[-1].strip() # get the new releasenote file path

    with open(new_releasenote, "w") as f:
        f.write("""prelude:
    |
    Release on: {1}

    - Please refer to the `{0} tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-{2}>`_ for the list of changes on the Core Checks\n""".format(version, date.today(), version.replace('.', '')))

    ctx.run("git add {}".format(new_releasenote))
    ctx.run("git commit -m \"Add prelude for {} release\"".format(version))

@task
def update_changelog(ctx, new_version):
    """
    Quick task to generate the new CHANGELOG using reno when releasing a minor
    version (linux/macOS only).
    """
    new_version_int = list(map(int, new_version.split(".")))

    if len(new_version_int) != 3:
        print("Error: invalid version: {}".format(new_version_int))
        raise Exit(1)

    # let's avoid losing uncommitted change with 'git reset --hard'
    try:
        ctx.run("git diff --exit-code HEAD", hide="both")
    except Failure as e:
        print("Error: You have uncommitted change, please commit or stash before using update_changelog")
        return

    # make sure we are up to date
    ctx.run("git fetch")

    # let's check that the tag for the new version is present (needed by reno)
    try:
        ctx.run("git tag --list | grep {}".format(new_version))
    except Failure as e:
        print("Missing '{}' git tag: mandatory to use 'reno'".format(new_version))
        raise

    # removing releasenotes from bugfix on the old minor.
    previous_minor = "%s.%s" % (new_version_int[0], new_version_int[1] - 1)
    if previous_minor == "7.15":
        previous_minor = "6.15" # 7.15 is the first release in the 7.x series
    log_result = ctx.run("git log {}.0...remotes/origin/{}.x --name-only | \
            grep releasenotes/notes/ || true".format(previous_minor, previous_minor))
    log_result = log_result.stdout.replace('\n', ' ').strip()
    if len(log_result) > 0:
        ctx.run("git rm --ignore-unmatch {}".format(log_result))

    # generate the new changelog
    ctx.run("reno report \
            --ignore-cache \
            --earliest-version {}.0 \
            --version {} \
            --no-show-source > /tmp/new_changelog.rst".format(previous_minor, new_version))

    # reseting git
    ctx.run("git reset --hard HEAD")

    # mac's `sed` has a different syntax for the "-i" paramter
    sed_i_arg = "-i"
    if sys.platform == 'darwin':
        sed_i_arg = "-i ''"
    # check whether there is a v6 tag on the same v7 tag, if so add the v6 tag to the release title
    v6_tag = ""
    if new_version_int[0] == 7:
        v6_tag = _find_v6_tag(ctx, new_version)
        if v6_tag:
            ctx.run("sed {0} -E 's#^{1}#{1} / {2}#' /tmp/new_changelog.rst".format(sed_i_arg, new_version, v6_tag))
    # remove the old header from the existing changelog
    ctx.run("sed {0} -e '1,4d' CHANGELOG.rst".format(sed_i_arg))

    # merging to CHANGELOG.rst
    ctx.run("cat CHANGELOG.rst >> /tmp/new_changelog.rst && mv /tmp/new_changelog.rst CHANGELOG.rst")

    # commit new CHANGELOG
    ctx.run("git add CHANGELOG.rst \
            && git commit -m \"Update CHANGELOG for {}\"".format(new_version))

@task
def _find_v6_tag(ctx, v7_tag):
    """
    Returns one of the v6 tags that point at the same commit as the passed v7 tag.
    If none are found, returns the empty string.
    """
    v6_tag = ""

    print("Looking for a v6 tag pointing to same commit as tag '{}'...".format(v7_tag))
    # Find commit at which the v7_tag points
    commit = ctx.run("git rev-list --max-count=1 {}".format(v7_tag), hide='out').stdout.strip()
    try:
        v6_tags = ctx.run("git tag --points-at {} | grep -E '^6\\.'".format(commit), hide='out').stdout.strip().split("\n")
    except Failure as e:
        print("Found no v6 tag pointing at same commit as '{}'.".format(v7_tag))
    else:
        v6_tag = v6_tags[0]
        if len(v6_tags) > 1:
            print("Found v6 tags '{}', picking {}'".format(v6_tags, v6_tag))
        else:
            print("Found v6 tag '{}'".format(v6_tag))

    return v6_tag
