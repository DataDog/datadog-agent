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

from tasks.libs.common.color import color_message
from tasks.utils import DEFAULT_BRANCH

from .libs.common.user_interactions import yes_no_question
from .libs.version import Version
from .modules import DEFAULT_MODULES

# Generic version regex. Aims to match:
# - X.Y.Z
# - X.Y.Z-rc.t
# - vX.Y(.Z) (security-agent-policies repo)
VERSION_RE = re.compile(r'(v)?(\d+)[.](\d+)([.](\d+))?(-rc\.(\d+))?')


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


#
# release.json manipulation invoke tasks section
#

##
## I/O functions
##


def _load_release_json():
    with open("release.json", "r") as release_json_stream:
        return json.load(release_json_stream, object_pairs_hook=OrderedDict)


def _save_release_json(release_json):
    with open("release.json", "w") as release_json_stream:
        # Note, no space after the comma
        json.dump(release_json, release_json_stream, indent=4, sort_keys=False, separators=(',', ': '))


##
## Utils
##


def _create_version_from_match(match):
    groups = match.groups()
    version = Version(
        major=int(groups[1]),
        minor=int(groups[2]),
        patch=int(groups[4]) if groups[4] and groups[4] != 0 else None,
        rc=int(groups[6]) if groups[6] and groups[6] != 0 else None,
        prefix=groups[0] if groups[0] else "",
    )
    return version


def _stringify_config(config_dict):
    """
    Takes a config dict of the following form:
    {
        "xxx_VERSION": Version(major: x, minor: y, patch: z, rc: t, prefix: "pre"),
        "xxx_HASH": "hashvalue",
        ...
    }

    and transforms all VERSIONs into their string representation (using the Version object's __str__).
    """
    return {key: str(value) for key, value in config_dict.items()}


def _query_github_api(auth_token, url):
    import requests

    # Basic auth doesn't seem to work with private repos, so we use token auth here
    headers = {"Authorization": "token {}".format(auth_token)}
    response = requests.get(url, headers=headers)
    return response


def build_compatible_version_re(allowed_major_versions, minor_version):
    """
    Returns a regex that matches only versions whose major version is
    in the provided list of allowed_major_versions, and whose minor version matches
    the provided minor version.
    """
    return re.compile(
        r'(v)?({})[.]({})([.](\d+))?(-rc\.(\d+))?'.format("|".join(allowed_major_versions), minor_version)
    )


##
## Base functions to fetch candidate versions on other repositories
##


def _get_highest_repo_version(auth, repo, version_prefix, version_re, allowed_major_versions=None):
    # If allowed_major_versions is not specified, search for all versions by using an empty
    # major version prefix.
    if not allowed_major_versions:
        allowed_major_versions = [""]

    highest_version = None

    for major_version in allowed_major_versions:
        url = "https://api.github.com/repos/DataDog/{}/git/matching-refs/tags/{}{}".format(
            repo, version_prefix, major_version
        )

        tags = _query_github_api(auth, url).json()

        for tag in tags:
            match = version_re.search(tag["ref"])
            if match:
                this_version = _create_version_from_match(match)
                if this_version > highest_version:
                    highest_version = this_version

        # The allowed_major_versions are listed in order of preference
        # If something matching a given major version exists, no need to
        # go through the next ones.
        if highest_version:
            break

    if not highest_version:
        raise Exit("Couldn't find any matching {} version.".format(repo), 1)

    return highest_version


def _get_highest_version_from_release_json(release_json, major_version, version_re, release_json_key=None):
    """
    If release_json_key is None, returns the highest version entry in release.json.
    If release_json_key is set, returns the entry for release_json_key of the highest version entry in release.json.
    """

    highest_version = None
    highest_component_version = None
    for key, value in release_json.items():
        match = version_re.match(key)
        if match:
            this_version = _create_version_from_match(match)
            if this_version > highest_version and this_version.major <= major_version:
                highest_version = this_version

                if release_json_key is not None:
                    match = version_re.match(value.get(release_json_key, ""))
                    if match:
                        highest_component_version = _create_version_from_match(match)
                    else:
                        print(
                            "{} does not have a valid {} ({}), ignoring".format(
                                this_version, release_json_key, value.get(release_json_key, "")
                            )
                        )

    if not highest_version:
        raise Exit("Couldn't find any matching {} version.".format(release_json_key), 1)

    if release_json_key is not None:
        return highest_component_version

    return highest_version


##
## Variables needed for the repository version fetch functions
##

# COMPATIBLE_MAJOR_VERSIONS lists the major versions of tags
# that can be used with a given Agent version
# This is here for compatibility and simplicity reasons, as in most repos
# we don't create both 6 and 7 tags for a combined Agent 6 & 7 release.
# The order matters, eg. when fetching matching tags for an Agent 6 entry,
# tags starting with 6 will be preferred to tags starting with 7.
COMPATIBLE_MAJOR_VERSIONS = {6: ["6", "7"], 7: ["7"]}


# Message templates for the below functions
# Defined here either because they're long and would make the code less legible,
# or because they're used multiple times.
DIFFERENT_TAGS_TEMPLATE = (
    "The latest version of {} ({}) does not match the latest version found in the release.json file ({})."
)
TAG_NOT_FOUND_TEMPLATE = "Couldn't find a(n) {} version compatible with the new Agent version entry {}"
RC_TAG_QUESTION_TEMPLATE = "The {} tag found is an RC tag: {}. Are you sure you want to use it?"
TAG_FOUND_TEMPLATE = "The {} tag is {}"


##
## Repository version fetch functions
## The following functions aim at returning the correct version to use for a given
## repository, after compatibility & user confirmations
## The version object returned by such functions should be ready to be used to fill
## the release.json entry.
##


def _fetch_dependency_repo_version(
    repo_name, new_agent_version, allowed_major_versions, compatible_version_re, github_token, check_for_rc
):
    """
    Fetches the latest tag on a given repository whose version scheme matches the one used for the Agent,
    with the following constraints:
    - the tag must have a major version that's in allowed_major_versions
    - the tag must match compatible_version_re (the main usage is to restrict the compatible tags to the
      ones with the same minor version as the Agent)?
    
    If check_for_rc is true, a warning will be emitted if the latest version that satisfies
    the constraints is an RC. User confirmation is then needed to check that this is desired.
    """

    version = _get_highest_repo_version(
        github_token, repo_name, new_agent_version.prefix, compatible_version_re, allowed_major_versions
    )

    if check_for_rc and version.is_rc():
        if not yes_no_question(RC_TAG_QUESTION_TEMPLATE.format(repo_name, version), "orange", False):
            raise Exit("Aborting release.json update.", 1)

    print(TAG_FOUND_TEMPLATE.format(repo_name, version))
    return version


def _confirm_independent_dependency_repo_version(repo, latest_version, highest_release_json_version):
    """
    Checks if the two versions of a repository we found (from release.json and from the available repo tags)
    are different. If they are, asks the user for confirmation before updating the version.
    """

    if latest_version == highest_release_json_version:
        return highest_release_json_version

    print(color_message(DIFFERENT_TAGS_TEMPLATE.format(repo, latest_version, highest_release_json_version), "orange"))
    if yes_no_question("Do you want to update {} to {}?".format(repo, latest_version), "orange", False):
        return latest_version

    return highest_release_json_version


def _fetch_independent_dependency_repo_version(
    repo_name, release_json, agent_major_version, github_token, release_json_key
):
    """
    Fetches the latest tag on a given repository whose version scheme doesn't match the one used for the Agent:
    - first, we get the latest version used in release entries of the matching Agent major version
    - then, we fetch the latest version available in the repository
    - if the above two versions are different, emit a warning and ask for user confirmation before updating the version.
    """

    highest_version = _get_highest_version_from_release_json(
        release_json, agent_major_version, VERSION_RE, release_json_key=release_json_key,
    )
    # NOTE: This assumes that the repository doesn't change the way it prefixes versions.
    version = _get_highest_repo_version(github_token, repo_name, highest_version.prefix, VERSION_RE)

    version = _confirm_independent_dependency_repo_version(repo_name, version, highest_version)
    print(TAG_FOUND_TEMPLATE.format(repo_name, version))

    return version


def _get_windows_ddnpm_release_json_info(
    release_json, agent_major_version, version_re, is_first_rc=False,
):
    """
    Gets the Windows NPM driver info from the previous entries in the release.json file.
    """

    # First RC should use the data from nightly section otherwise reuse the last RC info
    if is_first_rc:
        print("Using 'nightly' DDNPM values")
        release_json_version_data = release_json['nightly']
    else:
        highest_release_json_version = _get_highest_version_from_release_json(
            release_json, agent_major_version, version_re
        )
        print("Using '{}' DDNPM values".format(highest_release_json_version))
        release_json_version_data = release_json[str(highest_release_json_version)]

    win_ddnpm_driver = release_json_version_data['WINDOWS_DDNPM_DRIVER']
    win_ddnpm_version = release_json_version_data['WINDOWS_DDNPM_VERSION']
    win_ddnpm_shasum = release_json_version_data['WINDOWS_DDNPM_SHASUM']

    if win_ddnpm_driver not in ['release-signed', 'attestation-signed']:
        print("WARN: WINDOWS_DDNPM_DRIVER value '{}' is not valid".format(win_ddnpm_driver))

    print("The windows ddnpm version is {}".format(win_ddnpm_version))

    return win_ddnpm_driver, win_ddnpm_version, win_ddnpm_shasum


@task
def get_variable(_, name, version='nightly'):
    with open("release.json", "r") as release_json_stream:
        release_json = json.load(release_json_stream, object_pairs_hook=OrderedDict)
    print(release_json[version][name])


##
## release_json object update function
##


def _add_release_json_entry(
    release_json,
    new_version,
    integrations_version,
    omnibus_software_version,
    omnibus_ruby_version,
    jmxfetch_version,
    security_agent_policies_version,
    macos_build_version,
    windows_ddnpm_driver,
    windows_ddnpm_version,
    windows_ddnpm_shasum,
):
    """
    Adds a new entry to provided release_json object with the provided parameters, and returns the new release_json object.
    """
    import requests

    jmxfetch = requests.get(
        "https://oss.sonatype.org/service/local/repositories/releases/content/com/datadoghq/jmxfetch/{0}/jmxfetch-{0}-jar-with-dependencies.jar".format(
            jmxfetch_version,
        )
    ).content
    jmxfetch_sha256 = hashlib.sha256(jmxfetch).hexdigest()

    print("Jmxfetch's SHA256 is {}".format(jmxfetch_sha256))
    print("Windows DDNPM's SHA256 is {}".format(windows_ddnpm_shasum))

    new_version_config = OrderedDict()
    new_version_config["INTEGRATIONS_CORE_VERSION"] = integrations_version
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
    new_release_json[str(new_version)] = _stringify_config(new_version_config)

    # Then the rest of the versions
    for key, value in release_json.items():
        if key not in new_release_json:
            new_release_json[key] = value

    return new_release_json


##
## Main functions
##


def _update_release_json(release_json, new_version, github_token, check_for_rc=False):
    """
    Updates the provided release.json object by fetching compatible versions for all dependencies
    of the provided Agent version, constructing the new entry, adding it to the release.json object
    and returning it.
    """

    allowed_major_versions = COMPATIBLE_MAJOR_VERSIONS[new_version.major]

    # Part 1: repositories which follow the Agent version scheme

    # For repositories which follow the Agent version scheme, we want to only get
    # tags with the same minor version, to avoid problems when releasing a patch
    # version while a minor version release is ongoing.
    compatible_version_re = build_compatible_version_re(allowed_major_versions, new_version.minor)

    integrations_version = _fetch_dependency_repo_version(
        "integrations-core", new_version, allowed_major_versions, compatible_version_re, github_token, check_for_rc
    )

    omnibus_software_version = _fetch_dependency_repo_version(
        "omnibus-software", new_version, allowed_major_versions, compatible_version_re, github_token, check_for_rc
    )

    omnibus_ruby_version = _fetch_dependency_repo_version(
        "omnibus-ruby", new_version, allowed_major_versions, compatible_version_re, github_token, check_for_rc
    )

    macos_build_version = _fetch_dependency_repo_version(
        "datadog-agent-macos-build",
        new_version,
        allowed_major_versions,
        compatible_version_re,
        github_token,
        check_for_rc,
    )

    # Part 2: repositories which have their own version scheme
    jmxfetch_version = _fetch_independent_dependency_repo_version(
        "jmxfetch", release_json, new_version.major, github_token, "JMXFETCH_VERSION"
    )

    security_agent_policies_version = _fetch_independent_dependency_repo_version(
        "security-agent-policies", release_json, new_version.major, github_token, "SECURITY_AGENT_POLICIES_VERSION"
    )

    windows_ddnpm_driver, windows_ddnpm_version, windows_ddnpm_shasum = _get_windows_ddnpm_release_json_info(
        release_json, new_version.major, VERSION_RE, is_first_rc=(new_version.rc == 1)
    )

    # Add new entry to the release.json object and return it
    return _add_release_json_entry(
        release_json,
        new_version,
        integrations_version,
        omnibus_software_version,
        omnibus_ruby_version,
        jmxfetch_version,
        security_agent_policies_version,
        macos_build_version,
        windows_ddnpm_driver,
        windows_ddnpm_version,
        windows_ddnpm_shasum,
    )


@task
def finish(ctx, major_versions="6,7"):
    """
    Creates a new entry in the release.json file for the new version. Removes all the RC entries.

    Updates internal module dependencies with the new version.
    """

    if sys.version_info[0] < 3:
        print("Must use Python 3 for this task")
        return Exit(code=1)

    list_major_versions = major_versions.split(",")
    print("Finishing release for major version(s) {}".format(list_major_versions))

    list_major_versions = [int(x) for x in list_major_versions]

    github_token = os.environ.get('GITHUB_TOKEN')
    if github_token is None:
        print(
            "Error: set the GITHUB_TOKEN environment variable.\nYou can create one by going to"
            " https://github.com/settings/tokens. It should have at least the 'repo' permissions."
        )
        return Exit(code=1)

    release_json = _load_release_json()

    for major_version in list_major_versions:
        highest_version = _get_highest_version_from_release_json(release_json, major_version, VERSION_RE)

        # Set the new version
        new_version = highest_version.next_version(rc=False)
        print("Creating {}".format(new_version))

        # Update release.json object with the entry for the new version
        release_json = _update_release_json(release_json, new_version, github_token, check_for_rc=True)

        # Erase RCs after we're done processing the new entry
        while highest_version.is_rc():
            # In case we have skipped an RC in the file...
            try:
                release_json.pop(str(highest_version))
            finally:
                highest_version.rc = highest_version.rc - 1

    _save_release_json(release_json)

    # Update internal module dependencies
    update_modules(ctx, str(new_version))


@task
def create_rc(ctx, major_versions="6,7", patch_version=False):
    """
    Takes whatever version is the highest in release.json and adds a new RC to it.
    If there was no RC, creates one, and:
    - by default bumps the minor version.
    - if --patch-version is specified, bumps the patch version.
    
    If there was an RC, create RC + 1.

    Updates internal module dependencies with the new RC.
    """

    if sys.version_info[0] < 3:
        print("Must use Python 3 for this task")
        return Exit(code=1)

    list_major_versions = major_versions.split(",")
    print("Creating RC for agent version(s) {}".format(list_major_versions))

    list_major_versions = [int(x) for x in list_major_versions]

    github_token = os.environ.get('GITHUB_TOKEN')
    if github_token is None:
        print(
            "Error: set the GITHUB_TOKEN environment variable.\nYou can create one by going to"
            " https://github.com/settings/tokens. It should have at least the 'repo' permissions."
        )
        return Exit(code=1)

    release_json = _load_release_json()

    for major_version in list_major_versions:
        highest_version = _get_highest_version_from_release_json(release_json, major_version, VERSION_RE)

        if highest_version.is_rc():
            # We're already on an RC, only bump the RC version
            new_version = highest_version.next_version(rc=True)
        else:
            if patch_version:
                new_version = highest_version.next_version(bump_patch=True, rc=True)
            else:
                new_version = highest_version.next_version(bump_minor=True, rc=True)
        print("Creating {}".format(new_version))

        # Update release.json object with the entry for the new version
        release_json = _update_release_json(release_json, new_version, github_token, check_for_rc=False)

    _save_release_json(release_json)

    # Update internal module dependencies
    # Uses the last major version processed
    update_modules(ctx, str(new_version))


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
def tag_version(ctx, agent_version, commit="HEAD", verify=True, push=True, force=False):
    """
    Create tags for a given Datadog Agent version.
    The version should be given as an Agent 7 version.

    * --commit COMMIT will tag COMMIT with the tags (default HEAD)
    * --verify checks for correctness on the Agent version (on by default).
    * --push will push the tags to the origin remote (on by default).
    * --force will allow the task to overwrite existing tags. Needed to move existing tags (off by default).

    Examples:
    inv -e release.tag-version 7.27.0                 # Create tags and push them to origin
    inv -e release.tag-version 7.27.0-rc.3 --no-push  # Create tags locally; don't push them
    inv -e release.tag-version 7.29.0-rc.3 --force    # Create tags (overwriting existing tags with the same name), force-push them to origin
    """
    if verify:
        check_version(agent_version)

    force_option = ""
    if force:
        print(color_message("--force option enabled. This will allow the task to overwrite existing tags.", "orange"))
        result = yes_no_question("Please confirm the use of the --force option.", color="orange", default=False)
        if result:
            print("Continuing with the --force option.")
            force_option = " --force"
        else:
            print("Continuing without the --force option.")

    for module in DEFAULT_MODULES.values():
        if module.should_tag:
            for tag in module.tag(agent_version):
                ctx.run(
                    "git tag -m {tag} {tag} {commit}{force_option}".format(
                        tag=tag, commit=commit, force_option=force_option
                    )
                )
                print("Created tag {tag}".format(tag=tag))
                if push:
                    ctx.run("git push origin {tag}{force_option}".format(tag=tag, force_option=force_option))
                    print("Pushed tag {tag}".format(tag=tag))

    print("Created all tags for version {}".format(agent_version))
