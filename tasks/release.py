"""
Release helper tasks
"""
from __future__ import print_function
import os
import re
import sys
import json
import urllib
import hashlib
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


def _is_version_higher(version_1, version_2):
    if not version_2:
        return True
    if version_1["major"] < version_2["major"]:
        return False
    if version_1["minor"] < version_2["minor"]:
        return False
    if version_1["patch"] < version_2["patch"]:
        return False
    if version_1["rc"] < version_2["rc"]:
        return False
    return True


def _create_version_dict_from_match(match):
    groups = match.groups()
    version = {
        "major": int(groups[0]),
        "minor": int(groups[1]),
        "patch": int(groups[2]),
        "rc": 0
    }
    if groups[4]:
        version["rc"] = int(groups[4])
    return version


def _stringify_version(version_dict):
    version = "{}.{}.{}" \
        .format(version_dict["major"],
                version_dict["minor"],
                version_dict["patch"])
    if version_dict["rc"] is not 0:
        version = "{}-rc.{}".format(version, version_dict["rc"])
    return version


def _get_repo_rc_version(auth, repo, new_rc_version, version_re):
    response = urllib.urlopen("{}api.github.com/repos/DataDog/{}/git/matching-refs/tags/{}.{}.{}"
                              .format(auth,
                                      repo,
                                      new_rc_version["major"],
                                      new_rc_version["minor"],
                                      new_rc_version["patch"]))
    tags = json.load(response)
    highest_version = None
    for tag in tags:
        match = version_re.search(tag["ref"])
        if match:
            this_version = _create_version_dict_from_match(match)
            if _is_version_higher(this_version, highest_version):
                highest_version = this_version



    version = "{}.{}.{}" \
        .format(highest_version["major"],
                highest_version["minor"],
                highest_version["patch"])
    if highest_version["rc"] is not 0:
        version = "{}-rc.{}".format(version, highest_version["rc"])
    return version


@task
def create_rc(ctx, omnibus_ruby_version = "datadog-5.5.0"):

    """
    Creates new entry in the release.json file for a new RC.
    Looks at the current release.json to find the highest version released and create a new RC based on that.
    """

    github_token = os.environ.get('GITHUB_TOKEN')
    if github_token is None:
        print(
            "Error: set the GITHUB_TOKEN environment variable.\nYou can create one by going to"
            " https://github.com/settings/tokens. It should have at least the 'repo' permissions.")
        return Exit(code=1)

    version_re = re.compile('(\\d+)[.](\\d+)[.](\\d+)(-rc\\.(\\d+))?')

    auth = "https://{}:x-oauth-basic@".format(github_token)

    with open("release.json", "r") as release_json_stream:
        release_json = json.load(release_json_stream)

    highest_version = None
    highest_jmxfetch_version = None

    for key, value in release_json.items():
        match = version_re.match(key)
        if match:
            this_version = _create_version_dict_from_match(match)
            if _is_version_higher(this_version, highest_version):
                highest_version = this_version
                match = version_re.match(value["JMXFETCH_VERSION"])
                if match:
                    highest_jmxfetch_version = _create_version_dict_from_match(match)

    highest_version["rc"] = highest_version["rc"] + 1
    new_rc = _stringify_version(highest_version)
    print("Creating {}".format(new_rc))

    integration_rc = _get_repo_rc_version(auth, "integrations-core", highest_version, version_re)
    print("Integration-Core's highest tag is {}".format(integration_rc))

    omnibus_software_rc = _get_repo_rc_version(auth, "omnibus-software", highest_version, version_re)
    print("Omnibus-Software's highest tag is {}".format(omnibus_software_rc))

    jmxfetch_rc = _get_repo_rc_version(auth, "jmxfetch", highest_jmxfetch_version, version_re)
    print("Jmxfetch's highest tag is {}".format(jmxfetch_rc))

    jmxfetch = urllib.urlopen("https://bintray.com/datadog/datadog-maven/download_file?file_path=com%2Fdatadoghq%2Fjmxfetch%2F{}%2Fjmxfetch-{}-jar-with-dependencies.jar")
    jmxfetch_sha256 = hashlib.sha256(jmxfetch.read()).hexdigest()

    print("Jmxfetch's SHA256 is {}".format(jmxfetch_sha256))

    new_rc_config = {
        "INTEGRATIONS_CORE_VERSION": integration_rc,
        "OMNIBUS_SOFTWARE_VERSION": omnibus_software_rc,
        "OMNIBUS_RUBY_VERSION": omnibus_ruby_version,
        "JMXFETCH_VERSION": jmxfetch_rc,
        "JMXFETCH_HASH": jmxfetch_sha256
    }

    release_json[new_rc] = new_rc_config

    # We need to release Agent 6 as well
    highest_version["major"] = 6
    new_rc = _stringify_version(highest_version)
    release_json[new_rc] = new_rc_config

    with open("release.json", "w") as release_json_stream:
        json.dump(release_json, release_json_stream, indent=4, sort_keys=True)
