#
# release.json manipulation invoke tasks section
#
import json
from collections import OrderedDict

from invoke.exceptions import Exit

from tasks.libs.ciproviders.gitlab_api import get_buildimages_version, get_test_infra_def_version
from tasks.libs.common.constants import TAG_FOUND_TEMPLATE
from tasks.libs.common.git import get_default_branch, is_agent6
from tasks.libs.releasing.documentation import _stringify_config
from tasks.libs.releasing.version import (
    RELEASE_JSON_DEPENDENCIES,
    VERSION_RE,
    _fetch_dependency_repo_version,
    _get_release_version_from_release_json,
    build_compatible_version_re,
)
from tasks.libs.types.version import Version

# COMPATIBLE_MAJOR_VERSIONS lists the major versions of tags
# that can be used with a given Agent version
# This is here for compatibility and simplicity reasons, as in most repos
# we don't create both 6 and 7 tags for a combined Agent 6 & 7 release.
# The order matters, eg. when fetching matching tags for an Agent 6 entry,
# tags starting with 6 will be preferred to tags starting with 7.
COMPATIBLE_MAJOR_VERSIONS = {6: ["6", "7"], 7: ["7"]}
INTEGRATIONS_CORE_JSON_FIELD = "INTEGRATIONS_CORE_VERSION"
RELEASE_JSON_FIELDS_TO_UPDATE = [
    INTEGRATIONS_CORE_JSON_FIELD,
    "OMNIBUS_RUBY_VERSION",
]

UNFREEZE_REPO_AGENT = "datadog-agent"
INTERNAL_DEPS_REPOS = ["omnibus-ruby"]
DEPENDENT_REPOS = INTERNAL_DEPS_REPOS + ["integrations-core"]
ALL_REPOS = DEPENDENT_REPOS + [UNFREEZE_REPO_AGENT]
UNFREEZE_REPOS = INTERNAL_DEPS_REPOS + [UNFREEZE_REPO_AGENT] + ["datadog-agent-buildimages", "test-infra-definitions"]
DEFAULT_BRANCHES = {
    "omnibus-ruby": "datadog-5.5.0",
    "datadog-agent": "main",
    "datadog-agent-buildimages": get_buildimages_version(),
    "test-infra-definitions": get_test_infra_def_version(),
}
DEFAULT_BRANCHES_AGENT6 = {
    "omnibus-ruby": "6.53.x",
    "datadog-agent": "6.53.x",
}


def load_release_json():
    with open("release.json") as release_json_stream:
        return json.load(release_json_stream, object_pairs_hook=OrderedDict)


def _save_release_json(release_json):
    with open("release.json", "w") as release_json_stream:
        # Note, no space after the comma
        json.dump(release_json, release_json_stream, indent=4, sort_keys=False, separators=(',', ': '))
        release_json_stream.write('\n')


def _get_jmxfetch_release_json_info(release_json):
    """
    Gets the JMXFetch version info from the previous entries in the release.json file.
    """

    release_json_version_data = release_json[RELEASE_JSON_DEPENDENCIES]

    jmxfetch_version = release_json_version_data['JMXFETCH_VERSION']
    jmxfetch_shasum = release_json_version_data['JMXFETCH_HASH']

    print(f"The JMXFetch version is {jmxfetch_version}")

    return jmxfetch_version, jmxfetch_shasum


def _get_windows_release_json_info(release_json):
    """
    Gets the Windows NPM driver info from the previous entries in the release.json file.
    """
    release_json_version_data = release_json[RELEASE_JSON_DEPENDENCIES]

    win_ddnpm_driver, win_ddnpm_version, win_ddnpm_shasum = _get_windows_driver_info(release_json_version_data, 'DDNPM')
    win_ddprocmon_driver, win_ddprocmon_version, win_ddprocmon_shasum = _get_windows_driver_info(
        release_json_version_data, 'DDPROCMON'
    )

    return (
        win_ddnpm_driver,
        win_ddnpm_version,
        win_ddnpm_shasum,
        win_ddprocmon_driver,
        win_ddprocmon_version,
        win_ddprocmon_shasum,
    )


def _get_windows_driver_info(release_json_version_data, driver_name):
    """
    Gets the Windows driver info from the release.json version data.
    """
    driver_key = f'WINDOWS_{driver_name}_DRIVER'
    version_key = f'WINDOWS_{driver_name}_VERSION'
    shasum_key = f'WINDOWS_{driver_name}_SHASUM'

    driver_value = release_json_version_data[driver_key]
    version_value = release_json_version_data[version_key]
    shasum_value = release_json_version_data[shasum_key]

    if driver_value not in ['release-signed', 'attestation-signed']:
        print(f"WARN: {driver_key} value '{driver_value}' is not valid")

    print(f"The windows {driver_name.lower()} version is {version_value}")

    return driver_value, version_value, shasum_value


##
## release_json object update function
##


def _update_release_json_entry(
    release_json,
    integrations_version,
    omnibus_ruby_version,
    jmxfetch_version,
    jmxfetch_shasum,
    security_agent_policies_version,
    windows_ddnpm_driver,
    windows_ddnpm_version,
    windows_ddnpm_shasum,
    windows_ddprocmon_driver,
    windows_ddprocmon_version,
    windows_ddprocmon_shasum,
):
    """
    Adds a new entry to provided release_json object with the provided parameters, and returns the new release_json object.
    """

    print(f"Jmxfetch's SHA256 is {jmxfetch_shasum}")
    print(f"Windows DDNPM's SHA256 is {windows_ddnpm_shasum}")
    print(f"Windows DDPROCMON's SHA256 is {windows_ddprocmon_shasum}")

    new_version_config = OrderedDict()
    new_version_config["INTEGRATIONS_CORE_VERSION"] = integrations_version
    new_version_config["OMNIBUS_RUBY_VERSION"] = omnibus_ruby_version
    new_version_config["JMXFETCH_VERSION"] = jmxfetch_version
    new_version_config["JMXFETCH_HASH"] = jmxfetch_shasum
    new_version_config["SECURITY_AGENT_POLICIES_VERSION"] = security_agent_policies_version
    new_version_config["WINDOWS_DDNPM_DRIVER"] = windows_ddnpm_driver
    new_version_config["WINDOWS_DDNPM_VERSION"] = windows_ddnpm_version
    new_version_config["WINDOWS_DDNPM_SHASUM"] = windows_ddnpm_shasum
    new_version_config["WINDOWS_DDPROCMON_DRIVER"] = windows_ddprocmon_driver
    new_version_config["WINDOWS_DDPROCMON_VERSION"] = windows_ddprocmon_version
    new_version_config["WINDOWS_DDPROCMON_SHASUM"] = windows_ddprocmon_shasum

    # Necessary if we want to maintain the JSON order, so that humans don't get confused
    new_release_json = OrderedDict()

    # Add all versions from the old release.json
    for key, value in release_json.items():
        new_release_json[key] = value

    # Then update the entry
    new_release_json[RELEASE_JSON_DEPENDENCIES] = _stringify_config(new_version_config)

    return new_release_json


##
## Main functions
##


def _update_release_json(release_json, new_version: Version, max_version: Version):
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

    # If the new version is a final version, set the check_for_rc flag to true to warn if a dependency's version
    # is an RC.
    check_for_rc = not new_version.is_rc()

    integrations_version = _fetch_dependency_repo_version(
        "integrations-core",
        new_version,
        max_version,
        allowed_major_versions,
        compatible_version_re,
        check_for_rc,
    )

    omnibus_ruby_version = _fetch_dependency_repo_version(
        "omnibus-ruby",
        new_version,
        max_version,
        allowed_major_versions,
        compatible_version_re,
        check_for_rc,
    )

    # Part 2: repositories which have their own version scheme

    # jmxfetch version is updated directly by the AML team
    jmxfetch_version, jmxfetch_shasum = _get_jmxfetch_release_json_info(release_json)

    # security agent policies are updated directly by the CWS team
    security_agent_policies_version = _get_release_version_from_release_json(
        release_json, VERSION_RE, "SECURITY_AGENT_POLICIES_VERSION"
    )
    print(TAG_FOUND_TEMPLATE.format("security-agent-policies", security_agent_policies_version))

    (
        windows_ddnpm_driver,
        windows_ddnpm_version,
        windows_ddnpm_shasum,
        windows_ddprocmon_driver,
        windows_ddprocmon_version,
        windows_ddprocmon_shasum,
    ) = _get_windows_release_json_info(release_json)

    # Add new entry to the release.json object and return it
    return _update_release_json_entry(
        release_json,
        integrations_version,
        omnibus_ruby_version,
        jmxfetch_version,
        jmxfetch_shasum,
        security_agent_policies_version,
        windows_ddnpm_driver,
        windows_ddnpm_version,
        windows_ddnpm_shasum,
        windows_ddprocmon_driver,
        windows_ddprocmon_version,
        windows_ddprocmon_shasum,
    )


def update_release_json(new_version: Version, max_version: Version):
    """
    Updates the release entries in release.json to prepare the next RC or final build.
    """
    release_json = load_release_json()

    print(f"Updating release json for {new_version}")

    # Update release.json object with the entry for the new version
    release_json = _update_release_json(release_json, new_version, max_version)

    _save_release_json(release_json)


def _get_release_json_value(key):
    release_json = load_release_json()

    path = key.split('::')

    for element in path:
        if element not in release_json:
            raise Exit(code=1, message=f"Couldn't find '{key}' in release.json")

        release_json = release_json.get(element)

    return release_json


def set_new_release_branch(branch):
    rj = load_release_json()

    rj["base_branch"] = branch

    for field in RELEASE_JSON_FIELDS_TO_UPDATE:
        rj[RELEASE_JSON_DEPENDENCIES][field] = f"{branch}"

    _save_release_json(rj)


def set_current_milestone(milestone):
    rj = load_release_json()
    rj["current_milestone"] = milestone
    _save_release_json(rj)


def get_current_milestone():
    rj = load_release_json()
    return rj["current_milestone"]


def generate_repo_data(ctx, warning_mode, next_version, release_branch):
    if warning_mode:
        # Warning mode is used to warn integrations so that they prepare their release version in advance.
        repos = ["integrations-core"]
    elif next_version.major == 6:
        # For Agent 6, we are only concerned by datadog-agent. The other repos won't be updated.
        repos = ["datadog-agent"]
    else:
        repos = ALL_REPOS
    previous_tags = find_previous_tags(RELEASE_JSON_DEPENDENCIES, repos, RELEASE_JSON_FIELDS_TO_UPDATE)
    data = {}
    for repo in repos:
        branch = release_branch
        if branch == get_default_branch():
            branch = (
                next_version.branch()
                if repo == "integrations-core"
                else (DEFAULT_BRANCHES_AGENT6 if is_agent6(ctx) else DEFAULT_BRANCHES).get(repo, get_default_branch())
            )
        data[repo] = {
            'branch': branch,
            'previous_tag': previous_tags.get(repo, ""),
        }
    return data


def find_previous_tags(build, repos, all_keys):
    """
    Finds the previous tags for the given repositories in the release.json file.
    """
    tags = {}
    release_json = load_release_json()
    for key in all_keys:
        r = key.casefold().removesuffix("_version").replace("_", "-")
        repo = next((repo for repo in repos if r in repo), None)
        if repo:
            tags[repo] = release_json[build][key]
    return tags
