#
# release.json manipulation invoke tasks section
#
import json
import os
from collections import OrderedDict

from invoke.exceptions import Exit

from tasks.libs.ciproviders.gitlab_api import get_buildimages_version
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
DEFAULT_BRANCHES = {
    "omnibus-ruby": "datadog-5.5.0",
    "datadog-agent": "main",
    "datadog-agent-buildimages": get_buildimages_version(),
}
DEFAULT_BRANCHES_AGENT6 = {
    "omnibus-ruby": "6.53.x",
    "datadog-agent": "6.53.x",
}

# The release.json configuration is split across per-project shard files under
# release.d/, each with its own CODEOWNERS entry. This avoids cross-team merge
# conflicts and over-broad review requests when a single project updates its config.
# On load, all shards are recursively merged with release.json. On save, each
# key's shard residence is discovered by reading which shard currently contains it,
# preserving ownership structure automatically.
RELEASE_JSON = "release.json"
DEPENDENCY_SHARD_DIR = "release.d"


def _shard_path(filename):
    return os.path.join(DEPENDENCY_SHARD_DIR, filename)


def _iter_shard_files():
    """Yields (filename, parsed_shard) for each JSON file under release.d/, sorted by name."""
    for filename in sorted(os.listdir(DEPENDENCY_SHARD_DIR)):
        if not filename.endswith(".json"):
            continue
        with open(_shard_path(filename)) as shard_stream:
            yield filename, json.load(shard_stream, object_pairs_hook=OrderedDict)


def _dump_json(path, data):
    with open(path, "w") as stream:
        # Note, no space after the comma
        json.dump(data, stream, indent=4, sort_keys=False, separators=(',', ': '))
        stream.write('\n')


def _recursive_merge(base, override):
    """
    Recursively merge override dict into base dict. Override values win.
    Both are expected to be OrderedDicts to preserve order.
    """
    result = OrderedDict(base)
    for key, value in override.items():
        if key in result and isinstance(result[key], dict) and isinstance(value, dict):
            result[key] = _recursive_merge(result[key], value)
        else:
            result[key] = value
    return result


def load_release_json():
    """
    Loads release.json and recursively merges all per-project shard files found
    under release.d/, treating it like a conf.d-style configuration system.
    This allows shards to add or override any top-level or nested keys.
    """
    with open(RELEASE_JSON) as release_json_stream:
        release_json = json.load(release_json_stream, object_pairs_hook=OrderedDict)

    for filename in sorted(os.listdir(DEPENDENCY_SHARD_DIR)):
        if not filename.endswith(".json"):
            continue
        with open(_shard_path(filename)) as shard_stream:
            shard = json.load(shard_stream, object_pairs_hook=OrderedDict)
        release_json = _recursive_merge(release_json, shard)

    # Sort top-level dependencies block for deterministic output (if it exists).
    if RELEASE_JSON_DEPENDENCIES in release_json and isinstance(release_json[RELEASE_JSON_DEPENDENCIES], dict):
        release_json[RELEASE_JSON_DEPENDENCIES] = OrderedDict(sorted(release_json[RELEASE_JSON_DEPENDENCIES].items()))

    return release_json


def _save_release_json(release_json):
    """
    Splits the merged release_json back into release.json plus the shards under
    release.d/, preserving the conf.d layout. Each key is written back to
    whichever file currently owns it on disk (release.json or a specific shard);
    no hardcoded routing rules are used. A key that appears in no file on disk is
    a hard error, forcing an explicit owner assignment.

    Ownership is tracked at two levels: top-level keys, and one level of nesting
    (e.g. keys inside "dependencies"), which covers the current structure while
    leaving room for shards to grow additional keys.
    """
    merged = OrderedDict(release_json)

    shards = list(_iter_shard_files())

    # Determine which file owns each top-level key and each nested sub-key.
    # release.json is the base owner for anything not claimed by a shard.
    with open(RELEASE_JSON) as base_stream:
        base = json.load(base_stream, object_pairs_hook=OrderedDict)

    top_owner = {key: RELEASE_JSON for key in base}
    nested_owner = {}  # (top_key, sub_key) -> owning file
    for filename, shard in shards:
        for top_key, value in shard.items():
            if isinstance(value, dict):
                for sub_key in value:
                    nested_owner[(top_key, sub_key)] = filename
            else:
                top_owner[top_key] = filename

    def _new_content():
        return OrderedDict([(RELEASE_JSON, OrderedDict())] + [(filename, OrderedDict()) for filename, _ in shards])

    contents = _new_content()

    for top_key, value in merged.items():
        if isinstance(value, dict):
            # Route each sub-key to its owning file (defaulting the container's owner).
            for sub_key, sub_value in value.items():
                owner = nested_owner.get((top_key, sub_key)) or top_owner.get(top_key)
                if owner is None:
                    raise Exit(
                        code=1,
                        message=(
                            f"release.json key '{top_key}::{sub_key}' is not owned by release.json or any "
                            f"shard under {DEPENDENCY_SHARD_DIR}/. Create or update a shard to own it."
                        ),
                    )
                contents[owner].setdefault(top_key, OrderedDict())[sub_key] = sub_value
        else:
            owner = top_owner.get(top_key, RELEASE_JSON)
            contents[owner][top_key] = value

    # Sort nested dicts for deterministic output and write every file.
    for filename, data in contents.items():
        for key, value in data.items():
            if isinstance(value, dict):
                data[key] = OrderedDict(sorted(value.items()))
        _dump_json(filename if filename == RELEASE_JSON else _shard_path(filename), data)


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

    new_version_config = {}
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

    # TODO Agent Delivery: Check with AMP how we handle ADP in that file during the release process
    # Add all the other keys from the previous entry that are not explicitly set here
    for key in release_json[RELEASE_JSON_DEPENDENCIES]:
        if key not in new_version_config:
            new_version_config[key] = release_json[RELEASE_JSON_DEPENDENCIES][key]

    # Necessary if we want to maintain the JSON order, so that humans don't get confused
    new_release_json = OrderedDict()

    # Add all versions from the old release.json
    for key, value in release_json.items():
        new_release_json[key] = value

    # Then update the entry
    new_release_json[RELEASE_JSON_DEPENDENCIES] = _stringify_config(OrderedDict(sorted(new_version_config.items())))

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

    # jmxfetch version is updated directly by the AMP team
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
