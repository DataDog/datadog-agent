"""
Release helper tasks
"""


import hashlib
import json
import os
import re
import sys
from collections import OrderedDict
from datetime import date

from invoke import Failure, task
from invoke.exceptions import Exit

from tasks.utils import DEFAULT_BRANCH

from .modules import DEFAULT_MODULES


@task
def add_prelude(ctx, version):
    res = ctx.run("reno new prelude-release-{0}".format(version))
    new_releasenote = res.stdout.split(' ')[-1].strip()  # get the new releasenote file path

    with open(new_releasenote, "w") as f:
        f.write(
            """prelude:
    |
    Release on: {1}

    - Please refer to the `{0} tag on integrations-core <https://github.com/DataDog/integrations-core/blob/master/AGENT_CHANGELOG.md#datadog-agent-version-{2}>`_ for the list of changes on the Core Checks\n""".format(
                version, date.today(), version.replace('.', '')
            )
        )

    ctx.run("git add {}".format(new_releasenote))
    print("\nCommit this with:")
    print("git commit -m \"Add prelude for {} release\"".format(version))


@task
def add_dca_prelude(ctx, version, agent7_version, agent6_version=""):
    """
    Release of the Cluster Agent should be pinned to a version of the Agent.
    """
    res = ctx.run("reno --rel-notes-dir releasenotes-dca new prelude-release-{0}".format(version))
    new_releasenote = res.stdout.split(' ')[-1].strip()  # get the new releasenote file path

    if agent6_version != "":
        agent6_version = "--{}".format(
            agent6_version.replace('.', '')
        )  # generate the right hyperlink to the agent's changelog.

    with open(new_releasenote, "w") as f:
        f.write(
            """prelude:
    |
    Released on: {1}
    Pinned to datadog-agent v{0}: `CHANGELOG <https://github.com/DataDog/datadog-agent/blob/{4}/CHANGELOG.rst#{2}{3}>`_.""".format(
                agent7_version, date.today(), agent7_version.replace('.', ''), agent6_version, DEFAULT_BRANCH,
            )
        )

    ctx.run("git add {}".format(new_releasenote))
    print("\nCommit this with:")
    print("git commit -m \"Add prelude for {} release\"".format(version))


@task
def add_installscript_prelude(ctx, version):
    res = ctx.run("reno --rel-notes-dir releasenotes-installscript new prelude-release-{0}".format(version))
    new_releasenote = res.stdout.split(' ')[-1].strip()  # get the new releasenote file path

    with open(new_releasenote, "w") as f:
        f.write(
            """prelude:
    |
    Released on: {0}""".format(
                date.today()
            )
        )

    ctx.run("git add {}".format(new_releasenote))
    print("\nCommit this with:")
    print("git commit -m \"Add prelude for {} release\"".format(version))


@task
def update_dca_changelog(ctx, new_version, agent_version):
    """
    Quick task to generate the new CHANGELOG-DCA using reno when releasing a minor
    version (linux/macOS only).
    """
    new_version_int = list(map(int, new_version.split(".")))

    if len(new_version_int) != 3:
        print("Error: invalid version: {}".format(new_version_int))
        raise Exit(1)

    agent_version_int = list(map(int, agent_version.split(".")))

    if len(agent_version_int) != 3:
        print("Error: invalid version: {}".format(agent_version_int))
        raise Exit(1)

    # let's avoid losing uncommitted change with 'git reset --hard'
    try:
        ctx.run("git diff --exit-code HEAD", hide="both")
    except Failure:
        print("Error: You have uncommitted changes, please commit or stash before using update-dca-changelog")
        return

    # make sure we are up to date
    ctx.run("git fetch")

    # let's check that the tag for the new version is present (needed by reno)
    try:
        ctx.run("git tag --list | grep dca-{}".format(new_version))
    except Failure:
        print("Missing 'dca-{}' git tag: mandatory to use 'reno'".format(new_version))
        raise

    # Cluster agent minor releases are in sync with the agent's, bugfixes are not necessarily.
    # We rely on the agent's devel tag to enforce the sync between both releases.
    branching_point_agent = "{}.{}.0-devel".format(agent_version_int[0], agent_version_int[1])
    previous_minor_branchoff = "dca-{}.{}.X".format(new_version_int[0], new_version_int[1] - 1)
    log_result = ctx.run(
        "git log {}...remotes/origin/{} --name-only --oneline | \
            grep releasenotes-dca/notes/ || true".format(
            branching_point_agent, previous_minor_branchoff
        )
    )
    log_result = log_result.stdout.replace('\n', ' ').strip()

    # Do not include release notes that were added in the previous minor release branch (previous_minor_branchoff)
    # and the branch-off points for the current release (pined by the agent's devel tag)
    if len(log_result) > 0:
        ctx.run("git rm --ignore-unmatch {}".format(log_result))

    current_branchoff = "dca-{}.{}.X".format(new_version_int[0], new_version_int[1])
    # generate the new changelog. Specifying branch in case this is run outside the release branch that contains the tag.
    ctx.run(
        "reno --rel-notes-dir releasenotes-dca report \
            --ignore-cache \
            --branch {} \
            --version dca-{} \
            --no-show-source > /tmp/new_changelog-dca.rst".format(
            current_branchoff, new_version
        )
    )

    # reseting git
    ctx.run("git reset --hard HEAD")

    # mac's `sed` has a different syntax for the "-i" paramter
    sed_i_arg = "-i"
    if sys.platform == 'darwin':
        sed_i_arg = "-i ''"
    # remove the old header from the existing changelog
    ctx.run("sed {0} -e '1,4d' CHANGELOG-DCA.rst".format(sed_i_arg))

    if sys.platform != 'darwin':
        # sed on darwin doesn't support `-z`. On mac, you will need to manually update the following.
        ctx.run(
            "sed -z {0} -e 's/dca-{1}\\n===={2}/{1}\\n{2}/' /tmp/new_changelog-dca.rst".format(
                sed_i_arg, new_version, '=' * len(new_version)
            )
        )

    # merging to CHANGELOG.rst
    ctx.run("cat CHANGELOG-DCA.rst >> /tmp/new_changelog-dca.rst && mv /tmp/new_changelog-dca.rst CHANGELOG-DCA.rst")

    # commit new CHANGELOG
    ctx.run("git add CHANGELOG-DCA.rst")

    print("\nCommit this with:")
    print("git commit -m \"[DCA] Update CHANGELOG for {}\"".format(new_version))


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
        previous_minor = "6.15"  # 7.15 is the first release in the 7.x series
    log_result = ctx.run(
        "git log {}...remotes/origin/{}.x --name-only --oneline | \
            grep releasenotes/notes/ || true".format(
            branching_point, previous_minor
        )
    )
    log_result = log_result.stdout.replace('\n', ' ').strip()
    if len(log_result) > 0:
        ctx.run("git rm --ignore-unmatch {}".format(log_result))

    # generate the new changelog
    ctx.run(
        "reno report \
            --ignore-cache \
            --earliest-version {} \
            --version {} \
            --no-show-source > /tmp/new_changelog.rst".format(
            branching_point, new_version
        )
    )

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
    ctx.run("git add CHANGELOG.rst")

    print("\nCommit this with:")
    print("git commit -m \"[DCA] Update CHANGELOG for {}\"".format(new_version))


@task
def update_installscript_changelog(ctx, new_version):
    """
    Quick task to generate the new CHANGELOG-INSTALLSCRIPT using reno when releasing a minor
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
        print("Error: You have uncommitted changes, please commit or stash before using update-installscript-changelog")
        return

    # make sure we are up to date
    ctx.run("git fetch")

    # let's check that the tag for the new version is present (needed by reno)
    try:
        ctx.run("git tag --list | grep installscript-{}".format(new_version))
    except Failure:
        print("Missing 'installscript-{}' git tag: mandatory to use 'reno'".format(new_version))
        raise

    # generate the new changelog
    ctx.run(
        "reno --rel-notes-dir releasenotes-installscript report \
            --ignore-cache \
            --version installscript-{} \
            --no-show-source > /tmp/new_changelog-installscript.rst".format(
            new_version
        )
    )

    # reseting git
    ctx.run("git reset --hard HEAD")

    # mac's `sed` has a different syntax for the "-i" paramter
    sed_i_arg = "-i"
    if sys.platform == 'darwin':
        sed_i_arg = "-i ''"
    # remove the old header from the existing changelog
    ctx.run("sed {0} -e '1,4d' CHANGELOG-INSTALLSCRIPT.rst".format(sed_i_arg))

    if sys.platform != 'darwin':
        # sed on darwin doesn't support `-z`. On mac, you will need to manually update the following.
        ctx.run(
            "sed -z {0} -e 's/installscript-{1}\\n===={2}/{1}\\n{2}/' /tmp/new_changelog-installscript.rst".format(
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
    print("git commit -m \"[INSTALLSCRIPT] Update CHANGELOG-INSTALLSCRIPT for {}\"".format(new_version))


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
        v6_tags = (
            ctx.run("git tag --points-at {} | grep -E '^6\\.'".format(commit), hide='out').stdout.strip().split("\n")
        )
    except Failure:
        print("Found no v6 tag pointing at same commit as '{}'.".format(v7_tag))
    else:
        v6_tag = v6_tags[0]
        if len(v6_tags) > 1:
            print("Found v6 tags '{}', picking {}'".format(v6_tags, v6_tag))
        else:
            print("Found v6 tag '{}'".format(v6_tag))

    return v6_tag


@task
def list_major_change(_, milestone):
    """
    List all PR labeled "major_changed" for this release.
    """

    github_token = os.environ.get('GITHUB_TOKEN')
    if github_token is None:
        print(
            "Error: set the GITHUB_TOKEN environment variable.\nYou can create one by going to"
            " https://github.com/settings/tokens. It should have at least the 'repo' permissions."
        )
        return Exit(code=1)

    response = _query_github_api(
        github_token,
        "https://api.github.com/search/issues?q=repo:datadog/datadog-agent+label:major_change+milestone:{}".format(
            milestone
        ),
    )
    results = response.json()
    if not results["items"]:
        print("no major change for {}".format(milestone))
        return

    for pr in results["items"]:
        print("#{}: {} ({})".format(pr["number"], pr["title"], pr["html_url"]))


def _is_version_higher(version_1, version_2):
    if not version_2:
        return True

    for part in ["major", "minor", "patch"]:
        # Consider that a None version part is equivalent to a 0 version part
        version_1_part = version_1[part] if version_1[part] is not None else 0
        version_2_part = version_2[part] if version_2[part] is not None else 0

        if version_1_part != version_2_part:
            return version_1_part > version_2_part

    if version_1["rc"] is None or version_2["rc"] is None:
        # Everything else being equal, version_1 can only be higher than version_2 if version_2 is not a released version
        return version_2["rc"] is not None

    return version_1["rc"] > version_2["rc"]


def _create_version_dict_from_match(match):
    groups = match.groups()
    version = {
        "prefix": groups[0] if groups[0] else "",
        "major": int(groups[1]),
        "minor": int(groups[2]),
        "patch": int(groups[4]) if groups[4] and groups[4] != 0 else None,
        "rc": int(groups[6]) if groups[6] and groups[6] != 0 else None,
    }
    return version


def _is_dict_version_field(key):
    """
    Returns a bool to indicate if the field should be stringified from a dictionary or not.

    Generally all `*_VERSION` fields are parsed with regex but `WINDOWS_DDNPM_VERSION`
    should be used as-is.
    """
    return "VERSION" in key and key != "WINDOWS_DDNPM_VERSION"


def _stringify_config(config_dict):
    """
    Takes a config dict of the following form:
    {
        "xxx_VERSION": { "major": x, "minor": y, "patch": z, "rc": t },
        "xxx_HASH": "hashvalue",
        ...
    }

    and transforms all VERSIONs into their string representation.
    """
    return {
        key: _stringify_version(value) if _is_dict_version_field(key) else value for key, value in config_dict.items()
    }


def _stringify_version(version_dict):
    version = "{}{}.{}".format(version_dict["prefix"], version_dict["major"], version_dict["minor"])
    if version_dict["patch"] is not None:
        version = "{}.{}".format(version, version_dict["patch"])
    if version_dict["rc"] is not None and version_dict["rc"] != 0:
        version = "{}-rc.{}".format(version, version_dict["rc"])
    return version


def _query_github_api(auth_token, url):
    import requests

    # Basic auth doesn't seem to work with private repos, so we use token auth here
    headers = {"Authorization": "token {}".format(auth_token)}
    response = requests.get(url, headers=headers)
    return response


def _get_highest_repo_version(auth, repo, new_rc_version, version_re):
    if new_rc_version is not None:
        url = "https://api.github.com/repos/DataDog/{}/git/matching-refs/tags/{}{}".format(
            repo, new_rc_version["prefix"], new_rc_version["major"]
        )
    else:
        url = "https://api.github.com/repos/DataDog/{}/git/matching-refs/tags/".format(repo)

    response = _query_github_api(auth, url)
    tags = response.json()
    highest_version = None
    for tag in tags:
        match = version_re.search(tag["ref"])
        if match:
            this_version = _create_version_dict_from_match(match)
            if _is_version_higher(this_version, highest_version):
                highest_version = this_version
    return highest_version


def _get_highest_version_from_release_json(release_json, highest_major, version_re, release_json_key=None):
    """
    If release_json_key is None, returns the highest version entry in release.json.
    If release_json_key is set, returns the entry for release_json_key of the highest version entry in release.json.
    """

    highest_version = None
    highest_component_version = None
    for key, value in release_json.items():
        match = version_re.match(key)
        if match:
            this_version = _create_version_dict_from_match(match)
            if _is_version_higher(this_version, highest_version) and this_version["major"] <= highest_major:
                highest_version = this_version

                if release_json_key is not None:
                    match = version_re.match(value.get(release_json_key, ""))
                    if match:
                        highest_component_version = _create_version_dict_from_match(match)
                    else:
                        print(
                            "{} does not have a valid {} ({}), ignoring".format(
                                _stringify_version(this_version), release_json_key, value.get(release_json_key, "")
                            )
                        )

    if release_json_key is not None:
        return highest_component_version

    return highest_version


def _get_windows_ddnpm_release_json_info(
    release_json, highest_major, version_re, is_first_rc=False,
):

    highest_release_json_version = _get_highest_version_from_release_json(release_json, highest_major, version_re)

    # First RC should use the data from nightly section otherwise reuse the last RC info
    if is_first_rc:
        print("Using 'nightly' DDNPM values")
        release_json_version_data = release_json['nightly']
    else:
        highest_release = _stringify_version(highest_release_json_version)
        print("Using '{}' DDNPM values".format(highest_release))
        release_json_version_data = release_json[highest_release]

    win_ddnpm_driver = release_json_version_data['WINDOWS_DDNPM_DRIVER']
    win_ddnpm_version = release_json_version_data['WINDOWS_DDNPM_VERSION']
    win_ddnpm_shasum = release_json_version_data['WINDOWS_DDNPM_SHASUM']

    if win_ddnpm_driver not in ['release-signed', 'attestation-signed']:
        print("WARN: WINDOWS_DDNPM_DRIVER value '{}' is not valid".format(win_ddnpm_driver))

    return win_ddnpm_driver, win_ddnpm_version, win_ddnpm_shasum


def _save_release_json(
    release_json,
    list_major_versions,
    highest_version,
    integration_version,
    omnibus_software_version,
    omnibus_ruby_version,
    jmxfetch_version,
    security_agent_policies_version,
    macos_build_version,
    windows_ddnpm_driver,
    windows_ddnpm_version,
    windows_ddnpm_shasum,
):
    import requests

    jmxfetch = requests.get(
        "https://oss.sonatype.org/service/local/repositories/releases/content/com/datadoghq/jmxfetch/{0}/jmxfetch-{0}-jar-with-dependencies.jar".format(
            _stringify_version(jmxfetch_version),
        )
    ).content
    jmxfetch_sha256 = hashlib.sha256(jmxfetch).hexdigest()

    print("Jmxfetch's SHA256 is {}".format(jmxfetch_sha256))
    print("Windows DDNPM's SHA256 is {}".format(windows_ddnpm_shasum))

    new_version_config = OrderedDict()
    new_version_config["INTEGRATIONS_CORE_VERSION"] = integration_version
    new_version_config["OMNIBUS_SOFTWARE_VERSION"] = omnibus_software_version
    new_version_config["OMNIBUS_RUBY_VERSION"] = omnibus_ruby_version
    new_version_config["JMXFETCH_VERSION"] = jmxfetch_version
    new_version_config["JMXFETCH_HASH"] = jmxfetch_sha256
    new_version_config["SECURITY_AGENT_POLICIES_VERSION"] = security_agent_policies_version
    new_version_config["MACOS_BUILD_VERSION"] = macos_build_version
    new_version_config["WINDOWS_DDNPM_DRIVER"] = windows_ddnpm_driver
    new_version_config["WINDOWS_DDNPM_VERSION"] = windows_ddnpm_version
    new_version_config["WINDOWS_DDNPM_SHASUM"] = windows_ddnpm_shasum

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

        # Exception of datadog-agent-macos-build: we need one tag per major version
        new_version_config["MACOS_BUILD_VERSION"]["major"] = version
        new_release_json[new_version] = _stringify_config(new_version_config)

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
    major_versions="6,7",
    integration_version=None,
    omnibus_software_version=None,
    jmxfetch_version=None,
    omnibus_ruby_version=None,
    security_agent_policies_version=None,
    macos_build_version=None,
    windows_ddnpm_driver=None,
    windows_ddnpm_version=None,
    windows_ddnpm_shasum=None,
    ignore_rc_tag=False,
):

    """
    Creates new entry in the release.json file for the new version. Removes all the RC entries.

    Update internal module dependencies with the new version.
    """

    if sys.version_info[0] < 3:
        print("Must use Python 3 for this task")
        return Exit(code=1)

    list_major_versions = major_versions.split(",")
    print("Finishing release for major version(s) {}".format(list_major_versions))

    list_major_versions = [int(x) for x in list_major_versions]
    highest_major = 0
    for version in list_major_versions:
        if int(version) > highest_major:
            highest_major = version

    github_token = os.environ.get('GITHUB_TOKEN')
    if github_token is None:
        print(
            "Error: set the GITHUB_TOKEN environment variable.\nYou can create one by going to"
            " https://github.com/settings/tokens. It should have at least the 'repo' permissions."
        )
        return Exit(code=1)

    # We want to match:
    # - X.Y.Z
    # - X.Y.Z-rc.t
    # - vX.Y(.Z) (security-agent-policies repo)
    version_re = re.compile(r'(v)?(\d+)[.](\d+)([.](\d+))?(-rc\.(\d+))?')

    with open("release.json", "r") as release_json_stream:
        release_json = json.load(release_json_stream, object_pairs_hook=OrderedDict)

    highest_version = _get_highest_version_from_release_json(release_json, highest_major, version_re)

    # jmxfetch and security-agent-policies follow their own version scheme
    highest_jmxfetch_version = _get_highest_version_from_release_json(
        release_json, highest_major, version_re, release_json_key="JMXFETCH_VERSION",
    )

    highest_security_agent_policies_version = _get_highest_version_from_release_json(
        release_json, highest_major, version_re, release_json_key="SECURITY_AGENT_POLICIES_VERSION",
    )

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
            print(
                "ERROR: integrations-core tag is still an RC tag. That's probably NOT what you want in the final artifact."
            )
            if ignore_rc_tag:
                print("Continuing with RC tag on integrations-core.")
            else:
                print("Aborting.")
                return Exit(code=1)
    print("integrations-core's tag is {}".format(_stringify_version(integration_version)))

    if not omnibus_software_version:
        omnibus_software_version = _get_highest_repo_version(
            github_token, "omnibus-software", highest_version, version_re
        )
        if omnibus_software_version is None:
            print("ERROR: No version found for omnibus-software - did you create the tag?")
            return Exit(code=1)
        if omnibus_software_version["rc"] is not None:
            print(
                "ERROR: omnibus-software tag is still an RC tag. That's probably NOT what you want in the final artifact."
            )
            if ignore_rc_tag:
                print("Continuing with RC tag on omnibus-software.")
            else:
                print("Aborting.")
                return Exit(code=1)
    print("omnibus-software's tag is {}".format(_stringify_version(omnibus_software_version)))

    if not jmxfetch_version:
        jmxfetch_version = _get_highest_repo_version(github_token, "jmxfetch", highest_jmxfetch_version, version_re)
    print("jmxfetch's tag is {}".format(_stringify_version(jmxfetch_version)))

    if not omnibus_ruby_version:
        omnibus_ruby_version = _get_highest_repo_version(github_token, "omnibus-ruby", highest_version, version_re)
        if omnibus_ruby_version is None:
            print("ERROR: No version found for omnibus-ruby - did you create the tag?")
            return Exit(code=1)
        if omnibus_ruby_version["rc"] is not None:
            print(
                "ERROR: omnibus-ruby tag is still an RC tag. That's probably NOT what you want in the final artifact."
            )
            if ignore_rc_tag:
                print("Continuing with RC tag on omnibus-ruby.")
            else:
                print("Aborting.")
                return Exit(code=1)
    print("omnibus-ruby's tag is {}".format(_stringify_version(omnibus_ruby_version)))

    if not security_agent_policies_version:
        security_agent_policies_version = _get_highest_repo_version(
            github_token, "security-agent-policies", highest_security_agent_policies_version, version_re
        )
    print("security-agent-policies' tag is {}".format(_stringify_version(security_agent_policies_version)))

    if not macos_build_version:
        macos_build_version = _get_highest_repo_version(
            github_token, "datadog-agent-macos-build", highest_version, version_re
        )
        if macos_build_version is None:
            print("ERROR: No version found for datadog-agent-macos-build - did you create the tag?")
            return Exit(code=1)
        if macos_build_version["rc"] is not None:
            print(
                "ERROR: datadog-agent-macos-build tag is still an RC tag. That's probably NOT what you want in the final artifact."
            )
            if ignore_rc_tag:
                print("Continuing with RC tag on datadog-agent-macos-build.")
            else:
                print("Aborting.")
                return Exit(code=1)
    print("datadog-agent-macos-build' tag is {}".format(_stringify_version(macos_build_version)))

    if not windows_ddnpm_version:
        # Get info on DDNPM
        windows_ddnpm_driver, windows_ddnpm_version, windows_ddnpm_shasum = _get_windows_ddnpm_release_json_info(
            release_json, highest_major, version_re
        )

    print("windows ddnpm version is {}".format(windows_ddnpm_version))

    _save_release_json(
        release_json,
        list_major_versions,
        highest_version,
        integration_version,
        omnibus_software_version,
        omnibus_ruby_version,
        jmxfetch_version,
        security_agent_policies_version,
        macos_build_version,
        windows_ddnpm_driver,
        windows_ddnpm_version,
        windows_ddnpm_shasum,
    )

    # Update internal module dependencies
    update_modules(ctx, _stringify_version(highest_version))


@task
def create_rc(
    ctx,
    major_versions="6,7",
    integration_version=None,
    omnibus_software_version=None,
    jmxfetch_version=None,
    omnibus_ruby_version=None,
    security_agent_policies_version=None,
    macos_build_version=None,
    windows_ddnpm_driver=None,
    windows_ddnpm_version=None,
    windows_ddnpm_shasum=None,
):

    """
    Takes whatever version is the highest in release.json and adds a new RC to it.
    If there was no RC, creates one and bump minor version. If there was an RC, create RC + 1.

    Update internal module dependencies with the new RC.
    """

    if sys.version_info[0] < 3:
        print("Must use Python 3 for this task")
        return Exit(code=1)

    list_major_versions = major_versions.split(",")
    print("Creating RC for agent version(s) {}".format(list_major_versions))

    list_major_versions = [int(x) for x in list_major_versions]
    highest_major = 0
    for version in list_major_versions:
        if int(version) > highest_major:
            highest_major = version

    github_token = os.environ.get('GITHUB_TOKEN')
    if github_token is None:
        print(
            "Error: set the GITHUB_TOKEN environment variable.\nYou can create one by going to"
            " https://github.com/settings/tokens. It should have at least the 'repo' permissions."
        )
        return Exit(code=1)

    # We want to match:
    # - X.Y.Z
    # - X.Y.Z-rc.t
    # - vX.Y(.Z) (security-agent-policies repo)
    version_re = re.compile(r'(v)?(\d+)[.](\d+)([.](\d+))?(-rc\.(\d+))?')

    with open("release.json", "r") as release_json_stream:
        release_json = json.load(release_json_stream, object_pairs_hook=OrderedDict)

    highest_version = _get_highest_version_from_release_json(release_json, highest_major, version_re)

    # jmxfetch and security-agent-policies follow their own version scheme
    highest_jmxfetch_version = _get_highest_version_from_release_json(
        release_json, highest_major, version_re, release_json_key="JMXFETCH_VERSION",
    )

    highest_security_agent_policies_version = _get_highest_version_from_release_json(
        release_json, highest_major, version_re, release_json_key="SECURITY_AGENT_POLICIES_VERSION",
    )

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
    print("integrations-core's tag is {}".format(_stringify_version(integration_version)))

    if not omnibus_software_version:
        omnibus_software_version = _get_highest_repo_version(
            github_token, "omnibus-software", highest_version, version_re
        )
    print("omnibus-software's tag is {}".format(_stringify_version(omnibus_software_version)))

    if not jmxfetch_version:
        jmxfetch_version = _get_highest_repo_version(github_token, "jmxfetch", highest_jmxfetch_version, version_re)
    print("jmxfetch's tag is {}".format(_stringify_version(jmxfetch_version)))

    if not omnibus_ruby_version:
        omnibus_ruby_version = _get_highest_repo_version(github_token, "omnibus-ruby", highest_version, version_re)
    print("omnibus-ruby's tag is {}".format(_stringify_version(omnibus_ruby_version)))

    if not security_agent_policies_version:
        security_agent_policies_version = _get_highest_repo_version(
            github_token, "security-agent-policies", highest_security_agent_policies_version, version_re
        )
    print("security-agent-policies' tag is {}".format(_stringify_version(security_agent_policies_version)))

    if not macos_build_version:
        macos_build_version = _get_highest_repo_version(
            github_token, "datadog-agent-macos-build", highest_version, version_re
        )
    print("datadog-agent-macos-build's tag is {}".format(_stringify_version(macos_build_version)))

    if not windows_ddnpm_version:
        is_first_rc = highest_version["rc"] == 1
        # Get info on DDNPM
        windows_ddnpm_driver, windows_ddnpm_version, windows_ddnpm_shasum = _get_windows_ddnpm_release_json_info(
            release_json, highest_major, version_re, is_first_rc
        )

    print("windows ddnpm version is {}".format(windows_ddnpm_version))

    _save_release_json(
        release_json,
        list_major_versions,
        highest_version,
        integration_version,
        omnibus_software_version,
        omnibus_ruby_version,
        jmxfetch_version,
        security_agent_policies_version,
        macos_build_version,
        windows_ddnpm_driver,
        windows_ddnpm_version,
        windows_ddnpm_shasum,
    )

    # Update internal module dependencies
    update_modules(ctx, _stringify_version(highest_version))


def check_version(agent_version):
    """Check Agent version to see if it is valid."""
    version_re = re.compile(r'7[.](\d+)[.](\d+)(-rc\.(\d+))?')
    if not version_re.match(agent_version):
        raise Exit(message="Version should be of the form 7.Y.Z or 7.Y.Z-rc.t")


@task
def update_modules(ctx, agent_version, verify=True):
    """
    Update internal dependencies between the different Agent modules.
    * --verify checks for correctness on the Agent Version (on by default).

    Examples:
    inv -e release.update-modules 7.27.0
    """
    if verify:
        check_version(agent_version)

    for module in DEFAULT_MODULES.values():
        for dependency in module.dependencies:
            dependency_mod = DEFAULT_MODULES[dependency]
            ctx.run(
                "go mod edit -require={dependency_path} {go_mod_path}".format(
                    dependency_path=dependency_mod.dependency_path(agent_version), go_mod_path=module.go_mod_path()
                )
            )


@task
def tag_version(ctx, agent_version, commit="HEAD", verify=True, push=True):
    """
    Create tags for a given Datadog Agent version.
    The version should be given as an Agent 7 version.

    * --commit COMMIT will tag COMMIT with the tags (default HEAD)
    * --verify checks for correctness on the Agent version (on by default).
    * --push will push the tags to the origin remote (on by default).

    Examples:
    inv -e release.tag-version 7.27.0                 # Create tags and push them to origin
    inv -e release.tag-version 7.27.0-rc.3 --no-push  # Create tags locally; don't push them
    """
    if verify:
        check_version(agent_version)

    for module in DEFAULT_MODULES.values():
        if module.should_tag:
            for tag in module.tag(agent_version):
                ctx.run("git tag -m {tag} {tag} {commit}".format(tag=tag, commit=commit))
                if push:
                    ctx.run("git push origin {}".format(tag))

    print("Created all tags for version {}".format(agent_version))
