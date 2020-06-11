"""
Release helper tasks
"""
from __future__ import print_function
import os
import re
import sys
import json
import hashlib
from collections import OrderedDict
from datetime import date

from invoke import task, Failure
from invoke.exceptions import Exit


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
    except Failure:
        print("Error: You have uncommitted change, please commit or stash before using update_changelog")
        return

    # make sure we are up to date
    ctx.run("git fetch")

    # let's check that the tag for the new version is present (needed by reno)
    try:
        ctx.run("git tag --list | grep {}".format(new_version))
    except Failure:
        print("Missing '{}' git tag: mandatory to use 'reno'".format(new_version))
        raise

    # removing releasenotes from bugfix on the old minor.
    branching_point = "{}.{}.0-devel".format(new_version_int[0], new_version_int[1])
    previous_minor = "{}.{}".format(new_version_int[0], new_version_int[1] - 1)
    if previous_minor == "7.15":
        previous_minor = "6.15" # 7.15 is the first release in the 7.x series
    log_result = ctx.run("git log {}...remotes/origin/{}.x --name-only --oneline | \
            grep releasenotes/notes/ || true".format(branching_point, previous_minor))
    log_result = log_result.stdout.replace('\n', ' ').strip()
    if len(log_result) > 0:
        ctx.run("git rm --ignore-unmatch {}".format(log_result))

    # generate the new changelog
    ctx.run("reno report \
            --ignore-cache \
            --earliest-version {} \
            --version {} \
            --no-show-source > /tmp/new_changelog.rst".format(branching_point, new_version))

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
    except Failure:
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

    for part in ["major","minor","patch"]:
        if version_1[part] != version_2[part]:
            return version_1[part] > version_2[part]

    if version_1["rc"] is None or version_2["rc"] is None:
        # Everything else being equal, version_1 can only be higher than version_2 if version_2 is not a released version
        return version_2["rc"] is not None

    return version_1["rc"] > version_2["rc"]


def _create_version_dict_from_match(match):
    groups = match.groups()
    version = {
        "major": int(groups[0]),
        "minor": int(groups[1]),
        "patch": int(groups[2]),
        "rc": int(groups[4]) if groups[4] and groups[4] != 0 else None
    }
    return version


def _stringify_version(version_dict):
    version = "{}.{}.{}" \
        .format(version_dict["major"],
                version_dict["minor"],
                version_dict["patch"])
    if version_dict["rc"] is not None and version_dict["rc"] != 0:
        version = "{}-rc.{}".format(version, version_dict["rc"])
    return version


def _get_highest_repo_version(auth, repo, new_rc_version, version_re):
    import urllib.request
    password_mgr = urllib.request.HTTPPasswordMgrWithDefaultRealm()
    password_mgr.add_password(None, "api.github.com", auth, "x-oauth-basic")
    opener = urllib.request.build_opener(urllib.request.HTTPBasicAuthHandler(password_mgr))
    if new_rc_version is not None:
        response = opener.open("https://api.github.com/repos/DataDog/{}/git/matching-refs/tags/{}"
                               .format(repo, new_rc_version["major"]))
    else:
        response = opener.open("https://api.github.com/repos/DataDog/{}/git/matching-refs/tags/"
                               .format(repo))
    tags = json.load(response)
    highest_version = None
    for tag in tags:
        match = version_re.search(tag["ref"])
        if match:
            this_version = _create_version_dict_from_match(match)
            if _is_version_higher(this_version, highest_version):
                highest_version = this_version
    return highest_version


def _get_highest_version_from_release_json(release_json, highest_major, version_re):
    highest_version = None
    highest_jmxfetch_version = None
    for key, value in release_json.items():
        match = version_re.match(key)
        if match:
            this_version = _create_version_dict_from_match(match)
            if _is_version_higher(this_version, highest_version) and this_version["major"] <= highest_major:
                match = version_re.match(value["JMXFETCH_VERSION"])
                if match:
                    highest_jmxfetch_version = _create_version_dict_from_match(match)
                    highest_version = this_version
                else:
                    print("{} does not have a valid JMXFETCH_VERSION ({}), ignoring".format(_stringify_version(this_version), value["JMXFETCH_VERSION"]))
    return highest_version, highest_jmxfetch_version


def _save_release_json(release_json, list_major_versions, highest_version, integration_version, omnibus_software_version, omnibus_ruby_version, jmxfetch_version):
    import urllib.request
    jmxfetch = urllib.request.urlopen("https://bintray.com/datadog/datadog-maven/download_file?file_path=com%2Fdatadoghq%2Fjmxfetch%2F{}%2Fjmxfetch-{}-jar-with-dependencies.jar"
        .format(jmxfetch_version, jmxfetch_version))
    jmxfetch_sha256 = hashlib.sha256(jmxfetch.read()).hexdigest()

    print("Jmxfetch's SHA256 is {}".format(jmxfetch_sha256))

    new_version_config = OrderedDict()
    new_version_config["INTEGRATIONS_CORE_VERSION"] = integration_version
    new_version_config["OMNIBUS_SOFTWARE_VERSION"] = omnibus_software_version
    new_version_config["OMNIBUS_RUBY_VERSION"] = omnibus_ruby_version
    new_version_config["JMXFETCH_VERSION"] = jmxfetch_version
    new_version_config["JMXFETCH_HASH"] = jmxfetch_sha256

    # Necessary if we want to maintain the JSON order, so that humans don't get confused
    new_release_json = OrderedDict()

    # The nightlies should be at the top of the file
    nightly_version_re = re.compile('nightly(-a[0-9])?')
    for key, value in release_json.items():
        if nightly_version_re.match(key):
            new_release_json[key] = value

    # Then the new versions
    for version in list_major_versions:
        highest_version["major"] = version
        new_version = _stringify_version(highest_version)
        new_release_json[new_version] = new_version_config

    # Then the rest of the versions
    for key, value in release_json.items():
        if key not in new_release_json:
            new_release_json[key] = value

    with open("release.json", "w") as release_json_stream:
        # Note, no space after the comma
        json.dump(new_release_json, release_json_stream, indent=4, sort_keys=False, separators=(',', ': '))


@task
def finish(
    ctx,
    major_versions = "6,7",
    integration_version = None,
    omnibus_software_version = None,
    jmxfetch_version = None,
    omnibus_ruby_version = None,
    ignore_rc_tag = False):

    """
    Creates new entry in the release.json file for the new version. Removes all the RC entries.
    """

    if sys.version_info[0] < 3:
        print("Must use Python 3 for this task")
        return Exit(code=1)

    list_major_versions = major_versions.split(",")
    print("Finishing release for major version(s) {}".format(list_major_versions))

    list_major_versions = list(map(lambda x: int(x), list_major_versions))
    highest_major = 0
    for version in list_major_versions:
        if int(version) > highest_major:
            highest_major = version

    github_token = os.environ.get('GITHUB_TOKEN')
    if github_token is None:
        print(
            "Error: set the GITHUB_TOKEN environment variable.\nYou can create one by going to"
            " https://github.com/settings/tokens. It should have at least the 'repo' permissions.")
        return Exit(code=1)

    version_re = re.compile('(\\d+)[.](\\d+)[.](\\d+)(-rc\\.(\\d+))?')

    with open("release.json", "r") as release_json_stream:
        release_json = json.load(release_json_stream, object_pairs_hook=OrderedDict)

    highest_version, highest_jmxfetch_version = _get_highest_version_from_release_json(release_json, highest_major, version_re)

    # Erase RCs
    for major_version in list_major_versions:
        highest_version["major"] = major_version
        rc = highest_version["rc"]
        while highest_version["rc"] not in [0, None]:
            # In case we have skipped an RC in the file...
            try:
                release_json.pop(_stringify_version(highest_version))
            finally:
                highest_version["rc"] = highest_version["rc"] - 1
        highest_version["rc"] = rc

    # Tags in other repos are based on the highest major (e.g. for releasing version 6.X.Y and 7.X.Y they will tag only 7.X.Y)
    highest_version["major"] = highest_major

    # We don't want to fetch RC tags
    highest_version["rc"] = None

    if not integration_version:
        integration_version = _get_highest_repo_version(github_token, "integrations-core", highest_version, version_re)
        if integration_version is None:
            print("ERROR: No version found for integrations-core - did you create the tag?")
            return Exit(code=1)
        if integration_version["rc"] is not None:
            print("ERROR: integrations-core tag is still an RC tag. That's probably NOT what you want in the final artifact.")
            if ignore_rc_tag:
                print("Continuing with RC tag on integrations-core.")
            else:
                print("Aborting.")
                return Exit(code=1)
        integration_version = _stringify_version(integration_version)
    print("integrations-core's tag is {}".format(integration_version))

    if not omnibus_software_version:
        omnibus_software_version = _get_highest_repo_version(github_token, "omnibus-software", highest_version, version_re)
        if omnibus_software_version is None:
            print("ERROR: No version found for omnibus-software - did you create the tag?")
            return Exit(code=1)
        if omnibus_software_version["rc"] is not None:
            print("ERROR: omnibus-software tag is still an RC tag. That's probably NOT what you want in the final artifact.")
            if ignore_rc_tag:
                print("Continuing with RC tag on omnibus-software.")
            else:
                print("Aborting.")
                return Exit(code=1)
        omnibus_software_version = _stringify_version(omnibus_software_version)
    print("omnibus-software's tag is {}".format(omnibus_software_version))

    if not jmxfetch_version:
        jmxfetch_version = _get_highest_repo_version(github_token, "jmxfetch", highest_jmxfetch_version, version_re)
        jmxfetch_version = _stringify_version(jmxfetch_version)
    print("jmxfetch's tag is {}".format(jmxfetch_version))

    if not omnibus_ruby_version:
        omnibus_ruby_version = _get_highest_repo_version(github_token, "omnibus-ruby", highest_version, version_re)
        if omnibus_ruby_version is None:
            print("ERROR: No version found for omnibus-ruby - did you create the tag?")
            return Exit(code=1)
        if omnibus_ruby_version["rc"] is not None:
            print("ERROR: omnibus-ruby tag is still an RC tag. That's probably NOT what you want in the final artifact.")
            if ignore_rc_tag:
                print("Continuing with RC tag on omnibus-ruby.")
            else:
                print("Aborting.")
                return Exit(code=1)
        omnibus_ruby_version = _stringify_version(omnibus_ruby_version)
    print("omnibus-ruby's tag is {}".format(omnibus_ruby_version))

    _save_release_json(
        release_json,
        list_major_versions,
        highest_version,
        integration_version,
        omnibus_software_version,
        omnibus_ruby_version,
        jmxfetch_version)


@task
def create_rc(
    ctx,
    major_versions = "6,7",
    integration_version = None,
    omnibus_software_version = None,
    jmxfetch_version = None,
    omnibus_ruby_version = None):

    """
    Takes whatever version is the highest in release.json and adds a new RC to it.
    If there was no RC, creates one and bump minor version. If there was an RC, create RC + 1.
    """

    if sys.version_info[0] < 3:
        print("Must use Python 3 for this task")
        return Exit(code=1)

    list_major_versions = major_versions.split(",")
    print("Creating RC for agent version(s) {}".format(list_major_versions))

    list_major_versions = list(map(lambda x: int(x), list_major_versions))
    highest_major = 0
    for version in list_major_versions:
        if int(version) > highest_major:
            highest_major = version

    github_token = os.environ.get('GITHUB_TOKEN')
    if github_token is None:
        print(
            "Error: set the GITHUB_TOKEN environment variable.\nYou can create one by going to"
            " https://github.com/settings/tokens. It should have at least the 'repo' permissions.")
        return Exit(code=1)

    version_re = re.compile('(\\d+)[.](\\d+)[.](\\d+)(-rc\\.(\\d+))?')

    with open("release.json", "r") as release_json_stream:
        release_json = json.load(release_json_stream, object_pairs_hook=OrderedDict)

    highest_version, highest_jmxfetch_version = _get_highest_version_from_release_json(release_json, highest_major, version_re)

    if highest_version["rc"] is None:
        # No RC exists, create one
        highest_version["minor"] = highest_version["minor"] + 1
        highest_version["rc"] = 1
    else:
        # An RC exists, create next RC
        highest_version["rc"] = highest_version["rc"] + 1
    new_rc = _stringify_version(highest_version)
    print("Creating {}".format(new_rc))

    if not integration_version:
        integration_version = _get_highest_repo_version(github_token, "integrations-core", highest_version, version_re)
        integration_version = _stringify_version(integration_version)
    print("integrations-core's tag is {}".format(integration_version))

    if not omnibus_software_version:
        omnibus_software_version = _get_highest_repo_version(github_token, "omnibus-software", highest_version, version_re)
        omnibus_software_version = _stringify_version(omnibus_software_version)
    print("omnibus-software's tag is {}".format(omnibus_software_version))

    if not jmxfetch_version:
        jmxfetch_version = _get_highest_repo_version(github_token, "jmxfetch", highest_jmxfetch_version, version_re)
        jmxfetch_version = _stringify_version(jmxfetch_version)
    print("jmxfetch's tag is {}".format(jmxfetch_version))

    if not omnibus_ruby_version:
        omnibus_ruby_version = _get_highest_repo_version(github_token, "omnibus-ruby", highest_version, version_re)
        omnibus_ruby_version = _stringify_version(omnibus_ruby_version)
    print("omnibus-ruby's tag is {}".format(omnibus_ruby_version))

    _save_release_json(
        release_json,
        list_major_versions,
        highest_version,
        integration_version,
        omnibus_software_version,
        omnibus_ruby_version,
        jmxfetch_version)
