#
# release.json manipulation invoke tasks section
#
import json
from collections import OrderedDict

from invoke.exceptions import Exit

from tasks.libs.common.constants import TAG_FOUND_TEMPLATE
from tasks.libs.releasing.documentation import _stringify_config, nightly_entry_for, release_entry_for
from tasks.libs.releasing.version import (
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


def _load_release_json():
    with open("release.json") as release_json_stream:
        return json.load(release_json_stream, object_pairs_hook=OrderedDict)


def _save_release_json(release_json):
    with open("release.json", "w") as release_json_stream:
        # Note, no space after the comma
        json.dump(release_json, release_json_stream, indent=4, sort_keys=False, separators=(',', ': '))
        release_json_stream.write('\n')


def _get_jmxfetch_release_json_info(release_json, agent_major_version, is_first_rc=False):
    """
    Gets the JMXFetch version info from the previous entries in the release.json file.
    """

    release_json_version_data = _get_release_json_info_for_next_rc(release_json, agent_major_version, is_first_rc)

    jmxfetch_version = release_json_version_data['JMXFETCH_VERSION']
    jmxfetch_shasum = release_json_version_data['JMXFETCH_HASH']

    print(f"The JMXFetch version is {jmxfetch_version}")

    return jmxfetch_version, jmxfetch_shasum


def _get_windows_release_json_info(release_json, agent_major_version, is_first_rc=False):
    """
    Gets the Windows NPM driver info from the previous entries in the release.json file.
    """
    release_json_version_data = _get_release_json_info_for_next_rc(release_json, agent_major_version, is_first_rc)

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


def _get_release_json_info_for_next_rc(release_json, agent_major_version, is_first_rc=False):
    """
    Gets the version info from the previous entries in the release.json file.
    """

    # First RC should use the data from nightly section otherwise reuse the last RC info
    if is_first_rc:
        previous_release_json_version = nightly_entry_for(agent_major_version)
    else:
        previous_release_json_version = release_entry_for(agent_major_version)

    print(f"Using '{previous_release_json_version}' values")

    return release_json[previous_release_json_version]


##
## release_json object update function
##


def _update_release_json_entry(
    release_json,
    release_entry,
    integrations_version,
    omnibus_software_version,
    omnibus_ruby_version,
    jmxfetch_version,
    jmxfetch_shasum,
    security_agent_policies_version,
    macos_build_version,
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
    new_version_config["OMNIBUS_SOFTWARE_VERSION"] = omnibus_software_version
    new_version_config["OMNIBUS_RUBY_VERSION"] = omnibus_ruby_version
    new_version_config["JMXFETCH_VERSION"] = jmxfetch_version
    new_version_config["JMXFETCH_HASH"] = jmxfetch_shasum
    new_version_config["SECURITY_AGENT_POLICIES_VERSION"] = security_agent_policies_version
    new_version_config["MACOS_BUILD_VERSION"] = macos_build_version
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
    new_release_json[release_entry] = _stringify_config(new_version_config)

    return new_release_json


##
## Main functions
##


def _update_release_json(release_json, release_entry, new_version: Version, max_version: Version):
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

    omnibus_software_version = _fetch_dependency_repo_version(
        "omnibus-software",
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

    macos_build_version = _fetch_dependency_repo_version(
        "datadog-agent-macos-build",
        new_version,
        max_version,
        allowed_major_versions,
        compatible_version_re,
        check_for_rc,
    )

    # Part 2: repositories which have their own version scheme

    # jmxfetch version is updated directly by the AML team
    jmxfetch_version, jmxfetch_shasum = _get_jmxfetch_release_json_info(
        release_json, new_version.major, is_first_rc=(new_version.rc == 1)
    )

    # security agent policies are updated directly by the CWS team
    security_agent_policies_version = _get_release_version_from_release_json(
        release_json, new_version.major, VERSION_RE, "SECURITY_AGENT_POLICIES_VERSION"
    )
    print(TAG_FOUND_TEMPLATE.format("security-agent-policies", security_agent_policies_version))

    (
        windows_ddnpm_driver,
        windows_ddnpm_version,
        windows_ddnpm_shasum,
        windows_ddprocmon_driver,
        windows_ddprocmon_version,
        windows_ddprocmon_shasum,
    ) = _get_windows_release_json_info(release_json, new_version.major, is_first_rc=(new_version.rc == 1))

    # Add new entry to the release.json object and return it
    return _update_release_json_entry(
        release_json,
        release_entry,
        integrations_version,
        omnibus_software_version,
        omnibus_ruby_version,
        jmxfetch_version,
        jmxfetch_shasum,
        security_agent_policies_version,
        macos_build_version,
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
    release_json = _load_release_json()

    release_entry = release_entry_for(new_version.major)
    print(f"Updating {release_entry} for {new_version}")

    # Update release.json object with the entry for the new version
    release_json = _update_release_json(release_json, release_entry, new_version, max_version)

    _save_release_json(release_json)


def _get_release_json_value(key):
    release_json = _load_release_json()

    path = key.split('::')

    for element in path:
        if element not in release_json:
            raise Exit(code=1, message=f"Couldn't find '{key}' in release.json")

        release_json = release_json.get(element)

    return release_json
