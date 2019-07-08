"""
Release helper tasks
"""
from __future__ import print_function
import os
import sys
from datetime import date

from invoke import task, Failure
from invoke.exceptions import UnexpectedExit


@task
def add_prelude(ctx, version):
    res = ctx.run("reno new prelude-release-{0}".format(version))
    new_releasenote = res.stdout.split(' ')[-1].strip() # get the new releasenote file path

    with open(new_releasenote, "w") as f:
        f.write("""prelude:
    |
    Release on: {1}

    - Please refer to the `{0} tag on stackstate-agent-integrations <https://github.com/StackVista/stackstate-agent-integrations/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-{2}>`_ for the list of changes on the Core Checks.

    - Please refer to the `{0} tag on process-agent <https://github.com/DataDog/datadog-process-agent/releases/tag/{0}>`_ for the list of changes on the Process Agent.\n""".format(version, date.today(), version.replace('.', '')))

    ctx.run("git add {}".format(new_releasenote))
    ctx.run("git commit -m \"Add prelude for {} release\"".format(version))

@task
def update_changelog(ctx, new_version):
    """
    Quick task to generate the new CHANGELOG using reno when releasing a minor
    version (linux only).
    """
    new_version_int = map(int, new_version.split("."))

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
    try:
        log_result = ctx.run("git log {}.0...remotes/origin/{}.x --name-only | \
                grep releasenotes/notes/".format(previous_minor, previous_minor))
        ctx.run("git rm --ignore-unmatch {}".format(log_result.stdin))
    except UnexpectedExit:
        pass  # non-zero exit code means no matches - nothing to do

    # generate the new changelog
    ctx.run("reno report \
            --ignore-cache \
            --earliest-version {}.0 \
            --version {} \
            --no-show-source > /tmp/new_changelog.rst".format(previous_minor, new_version))

    # reseting git
    ctx.run("git reset --hard HEAD")

    # remove the old header. Mac don't have the same sed CLI
    if sys.platform == 'darwin':
        ctx.run("sed -i '' -e '1,4d' CHANGELOG.rst")
    else:
        ctx.run("sed -i -e '1,4d' CHANGELOG.rst")

    # merging to CHANGELOG.rst
    ctx.run("cat CHANGELOG.rst >> /tmp/new_changelog.rst && mv /tmp/new_changelog.rst CHANGELOG.rst")

    # commit new CHANGELOG
    ctx.run("git add CHANGELOG.rst \
            && git commit -m \"Update CHANGELOG for {}\"".format(new_version))

@task
def generate_install(ctx, test_repo=False):
    """
    Task to generate Agent install.sh script that will use either the official or test debian repository
    """
    deb_official_repo = os.environ.get("STS_AWS_RELEASE_BUCKET")
    deb_test_repo = os.environ.get("STS_AWS_TEST_BUCKET")
    deb_repo = deb_test_repo if test_repo else deb_official_repo
    yum_official_repo = os.environ.get("STS_AWS_RELEASE_BUCKET_YUM")
    yum_test_repo = os.environ.get("STS_AWS_TEST_BUCKET_YUM")
    yum_repo = yum_test_repo if test_repo else yum_official_repo
    win_official_repo = os.environ.get("STS_AWS_RELEASE_BUCKET_WIN")
    win_test_repo = os.environ.get("STS_AWS_TEST_BUCKET_WIN")
    win_repo = win_test_repo if test_repo else win_official_repo
    print("Generating install.sh and install.ps1 ...")
    ctx.run("sed -e 's/$DEBIAN_REPO/https:\/\/{0}.s3.amazonaws.com/g' ./cmd/agent/install_script.sh > ./cmd/agent/install_1.sh".format(deb_repo))
    ctx.run("sed -e 's/$YUM_REPO/https:\/\/{0}.s3.amazonaws.com/g' ./cmd/agent/install_1.sh > ./cmd/agent/install.sh".format(yum_repo))
    ctx.run("rm ./cmd/agent/install_1.sh")
    ctx.run("sed -e 's/$env:WIN_REPO/https:\/\/{0}.s3.amazonaws.com\/windows/g' ./cmd/agent/install_script.ps1 > ./cmd/agent/install.ps1".format(win_repo))
