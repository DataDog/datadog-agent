import json
import os
import re
import sys

from invoke import Exit

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.constants import (
    AGENT_VERSION_CACHE_NAME,
    ALLOWED_REPO_NIGHTLY_BRANCHES,
    RC_TAG_QUESTION_TEMPLATE,
    REPO_NAME,
    TAG_FOUND_TEMPLATE,
)
from tasks.libs.common.git import get_commit_sha, get_current_branch
from tasks.libs.common.user_interactions import yes_no_question
from tasks.libs.releasing.documentation import release_entry_for
from tasks.libs.types.version import Version

# Generic version regex. Aims to match:
# - X.Y.Z
# - X.Y.Z-rc.t
# - X.Y.Z-devel
# - vX.Y(.Z) (security-agent-policies repo)
VERSION_RE = re.compile(r'(v)?(\d+)[.](\d+)([.](\d+))?(-devel)?(-rc\.(\d+))?')

# Regex matching rc version tag format like 7.50.0-rc.1
RC_VERSION_RE = re.compile(r'\d+[.]\d+[.]\d+-rc\.\d+')

# Regex matching minor release rc version tag like x.y.0-rc.1 (semver PATCH == 0), but not x.y.1-rc.1 (semver PATCH > 0)
MINOR_RC_VERSION_RE = re.compile(r'\d+[.]\d+[.]0-rc\.\d+')


def build_compatible_version_re(allowed_major_versions, minor_version):
    """
    Returns a regex that matches only versions whose major version is
    in the provided list of allowed_major_versions, and whose minor version matches
    the provided minor version.
    """
    return re.compile(
        r'(v)?({})[.]({})([.](\d+))+(-devel)?(-rc\.(\d+))?(?!-\w)'.format(  # noqa: FS002
            "|".join(allowed_major_versions), minor_version
        )
    )


def _create_version_from_match(match):
    groups = match.groups()
    version = Version(
        major=int(groups[1]),
        minor=int(groups[2]),
        patch=int(groups[4]) if groups[4] and groups[4] != 0 else None,
        devel=True if groups[5] else False,
        rc=int(groups[7]) if groups[7] and groups[7] != 0 else None,
        prefix=groups[0] if groups[0] else "",
    )
    return version


def check_version(agent_version):
    """Check Agent version to see if it is valid."""
    version_re = re.compile(r'7[.](\d+)[.](\d+)(-rc\.(\d+))?')
    if not version_re.match(agent_version):
        raise Exit(message="Version should be of the form 7.Y.Z or 7.Y.Z-rc.t")


def current_version(ctx, major_version) -> Version:
    return _create_version_from_match(VERSION_RE.search(get_version(ctx, major_version=major_version, release=True)))


def next_final_version(ctx, major_version, patch_version) -> Version:
    previous_version = current_version(ctx, major_version)

    # Set the new version
    if previous_version.is_devel():
        # If the previous version was a devel version, use the same version without devel
        # (should never happen during regular releases, we always do at least one RC)
        return previous_version.non_devel_version()
    if previous_version.is_rc():
        # If the previous version was an RC version, use the same version without RC
        return previous_version.next_version(rc=False)

    # Else, the latest version was a final release, so we use the next release
    # (eg. 7.32.1 from 7.32.0).
    if patch_version:
        return previous_version.next_version(bump_patch=True, rc=False)
    else:
        return previous_version.next_version(bump_minor=True, rc=False)


def next_rc_version(ctx, major_version, patch_version=False) -> Version:
    # Fetch previous version from the most recent tag on the branch
    previous_version = current_version(ctx, major_version)

    if previous_version.is_rc():
        # We're already on an RC, only bump the RC version
        new_version = previous_version.next_version(rc=True)
    else:
        if patch_version:
            new_version = previous_version.next_version(bump_patch=True, rc=True)
        else:
            # Minor version bump, we're doing a standard release:
            # - if the previous tag is a devel tag, use it without the devel tag
            # - otherwise (should not happen during regular release cycles), bump the minor version
            if previous_version.is_devel():
                new_version = previous_version.non_devel_version()
                new_version = new_version.next_version(rc=True)
            else:
                new_version = previous_version.next_version(bump_minor=True, rc=True)

    return new_version


def parse_major_versions(major_versions):
    return sorted(int(x) for x in major_versions.split(","))


##
## Repository version fetch functions
## The following functions aim at returning the correct version to use for a given
## repository, after compatibility & user confirmations
## The version object returned by such functions should be ready to be used to fill
## the release.json entry.
##


def _fetch_dependency_repo_version(
    repo_name,
    new_agent_version,
    max_agent_version,
    allowed_major_versions,
    compatible_version_re,
    check_for_rc,
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

    # Get the highest repo version that's not higher than the Agent version we're going to build
    # We don't want to use a tag on dependent repositories that is supposed to be used in a future
    # release of the Agent (eg. if 7.31.1-rc.1 is tagged on integrations-core while we're releasing 7.30.0).
    version = _get_highest_repo_version(
        repo_name,
        new_agent_version.prefix,
        compatible_version_re,
        allowed_major_versions,
        max_version=max_agent_version,
    )

    if check_for_rc and version.is_rc():
        if not yes_no_question(RC_TAG_QUESTION_TEMPLATE.format(repo_name, version), "orange", False):
            raise Exit("Aborting release.json update.", 1)

    print(TAG_FOUND_TEMPLATE.format(repo_name, version))
    return version


def _get_highest_repo_version(
    repo, version_prefix, version_re, allowed_major_versions=None, max_version: Version = None
):
    # If allowed_major_versions is not specified, search for all versions by using an empty
    # major version prefix.
    if not allowed_major_versions:
        allowed_major_versions = [""]

    highest_version = None

    gh = GithubAPI(repository=f'Datadog/{repo}')

    for major_version in allowed_major_versions:
        tags = gh.get_tags(f'{version_prefix}{major_version}')

        for tag in tags:
            match = version_re.search(tag.name)
            if match:
                this_version = _create_version_from_match(match)
                if max_version:
                    # Get the max version that corresponds to the major version
                    # of the current tag
                    this_max_version = max_version.clone()
                    this_max_version.major = this_version.major
                    if this_version > this_max_version:
                        continue
                if this_version > highest_version:
                    highest_version = this_version

        # The allowed_major_versions are listed in order of preference
        # If something matching a given major version exists, no need to
        # go through the next ones.
        if highest_version:
            break

    if not highest_version:
        raise Exit(f"Couldn't find any matching {repo} version.", 1)

    return highest_version


def _get_release_version_from_release_json(release_json, major_version, version_re, release_json_key=None):
    """
    If release_json_key is None, returns the highest version entry in release.json.
    If release_json_key is set, returns the entry for release_json_key of the highest version entry in release.json.
    """

    release_version = None
    release_component_version = None

    # Get the release entry for the given Agent major version
    release_entry_name = release_entry_for(major_version)
    release_json_entry = release_json.get(release_entry_name, None)

    # Check that the release entry exists, otherwise fail
    if release_json_entry:
        release_version = release_entry_name

        # Check that the component's version is defined in the release entry
        if release_json_key is not None:
            match = version_re.match(release_json_entry.get(release_json_key, ""))
            if match:
                release_component_version = _create_version_from_match(match)
            else:
                print(
                    f"{release_entry_name} does not have a valid {release_json_key} ({release_json_entry.get(release_json_key, '')}), ignoring"
                )

    if not release_version:
        raise Exit(f"Couldn't find any matching {release_version} version.", 1)

    if release_json_key is not None:
        return release_component_version

    return release_version


def get_version(
    ctx,
    url_safe=False,
    git_sha_length=7,
    major_version='7',
    include_pipeline_id=False,
    pipeline_id=None,
    include_git=False,
    include_pre=True,
    release=False,
):
    version = ""
    if pipeline_id is None:
        pipeline_id = os.getenv("CI_PIPELINE_ID")

    project_name = os.getenv("CI_PROJECT_NAME")
    try:
        agent_version_cache_file_exist = os.path.exists(AGENT_VERSION_CACHE_NAME)
        if not agent_version_cache_file_exist:
            if pipeline_id and pipeline_id.isdigit() and project_name == REPO_NAME:
                ctx.run(
                    f"aws s3 cp s3://dd-ci-artefacts-build-stable/datadog-agent/{pipeline_id}/{AGENT_VERSION_CACHE_NAME} .",
                    hide="stdout",
                )
                agent_version_cache_file_exist = True

        if agent_version_cache_file_exist:
            with open(AGENT_VERSION_CACHE_NAME) as file:
                cache_data = json.load(file)

            version, pre, commits_since_version, git_sha, pipeline_id = cache_data[major_version]
            # Dev's versions behave the same as nightly
            is_nightly = cache_data["nightly"] or cache_data["dev"]

            if pre and include_pre:
                version = f"{version}-{pre}"
    except (OSError, json.JSONDecodeError, IndexError) as e:
        # If a cache file is found but corrupted we ignore it.
        print(f"Error while recovering the version from {AGENT_VERSION_CACHE_NAME}: {e}", file=sys.stderr)
        version = ""
    # If we didn't load the cache
    if not version:
        if pipeline_id:
            # only log this warning in CI
            print("[WARN] Agent version cache file hasn't been loaded !", file=sys.stderr)
        # we only need the git info for the non omnibus builds, omnibus includes all this information by default
        version, pre, commits_since_version, git_sha, pipeline_id = query_version(
            ctx, major_version, git_sha_length=git_sha_length, release=release
        )
        # Dev's versions behave the same as nightly
        bucket_branch = os.getenv("BUCKET_BRANCH")
        is_nightly = bucket_branch in ALLOWED_REPO_NIGHTLY_BRANCHES or bucket_branch == "dev"
        if pre and include_pre:
            version = f"{version}-{pre}"

    if not commits_since_version and is_nightly and include_git:
        if url_safe:
            version = f"{version}.git.{0}.{git_sha}"
        else:
            version = f"{version}+git.{0}.{git_sha}"

    if commits_since_version and include_git:
        if url_safe:
            version = f"{version}.git.{commits_since_version}.{git_sha}"
        else:
            version = f"{version}+git.{commits_since_version}.{git_sha}"

    if is_nightly and include_git and include_pipeline_id and pipeline_id is not None:
        version = f"{version}.pipeline.{pipeline_id}"

    # version could be unicode as it comes from `query_version`
    return str(version)


def get_version_numeric_only(ctx, major_version='7'):
    # we only need the git info for the non omnibus builds, omnibus includes all this information by default
    version = ""
    pipeline_id = os.getenv("CI_PIPELINE_ID")
    project_name = os.getenv("CI_PROJECT_NAME")
    if pipeline_id and pipeline_id.isdigit() and project_name == REPO_NAME:
        try:
            if not os.path.exists(AGENT_VERSION_CACHE_NAME):
                ctx.run(
                    f"aws s3 cp s3://dd-ci-artefacts-build-stable/datadog-agent/{pipeline_id}/{AGENT_VERSION_CACHE_NAME} .",
                    hide="stdout",
                )

            with open(AGENT_VERSION_CACHE_NAME) as file:
                cache_data = json.load(file)

            version, *_ = cache_data[major_version]
        except (OSError, json.JSONDecodeError, IndexError) as e:
            # If a cache file is found but corrupted we ignore it.
            print(f"Error while recovering the version from {AGENT_VERSION_CACHE_NAME}: {e}")
            version = ""
    if not version:
        version, *_ = query_version(ctx, major_version)
    return version


def load_release_versions(_, target_version):
    with open("release.json") as f:
        versions = json.load(f)
        if target_version in versions:
            # windows runners don't accepts anything else than strings in the
            # environment when running a subprocess.
            return {str(k): str(v) for k, v in versions[target_version].items()}
    raise Exception(f"Could not find '{target_version}' version in release.json")


def create_version_json(ctx, git_sha_length=7):
    """
    Generate a json cache file containing all needed variables used by get_version.
    """
    packed_data = {}
    for maj_version in ['6', '7']:
        version, pre, commits_since_version, git_sha, pipeline_id = query_version(
            ctx, maj_version, git_sha_length=git_sha_length
        )
        packed_data[maj_version] = [version, pre, commits_since_version, git_sha, pipeline_id]
    bucket_branch = os.getenv("BUCKET_BRANCH")
    packed_data["nightly"] = bucket_branch in ALLOWED_REPO_NIGHTLY_BRANCHES
    packed_data["dev"] = bucket_branch == "dev"
    with open(AGENT_VERSION_CACHE_NAME, "w") as file:
        json.dump(packed_data, file, indent=4)


def query_version(ctx, major_version, git_sha_length=7, release=False):
    # The describe string format is <tag>-<number of commits since the tag>-g<commit hash>
    # e.g. 6.0.0-beta.0-1-g4f19118
    #   - tag is 6.0.0-beta.0
    #   - it has been one commit since the tag creation
    #   - that commit hash is g4f19118
    cmd = rf'git describe --tags --candidates=50 --match "{get_matching_pattern(ctx, major_version, release=release)}"'
    if git_sha_length and isinstance(git_sha_length, int):
        cmd += f" --abbrev={git_sha_length}"
    described_version = ctx.run(cmd, hide=True).stdout.strip()

    # for the example above, 6.0.0-beta.0-1-g4f19118, this will be 1
    commit_number_match = re.match(r"^.*-(?P<commit_number>\d+)-g[0-9a-f]+$", described_version)
    commit_number = 0
    if commit_number_match:
        commit_number = int(commit_number_match.group('commit_number'))

    version_re = r"^v?(?P<version>\d+\.\d+\.\d+)(?:(?:-|\.)(?P<pre>[0-9A-Za-z.-]+))?"
    if commit_number == 0:
        version_re += r"(?P<git_sha>)$"
    else:
        version_re += r"-\d+-g(?P<git_sha>[0-9a-f]+)$"

    version_match = re.match(version_re, described_version)

    if not version_match:
        raise Exception("Could not query valid version from tags of local git repository")

    # version: for the tag 6.0.0-beta.0, this will match 6.0.0
    # pre: for the output, 6.0.0-beta.0-1-g4f19118, this will match beta.0
    # if there have been no commits since, it will be just 6.0.0-beta.0,
    # and it will match beta.0
    # git_sha: for the output, 6.0.0-beta.0-1-g4f19118, this will match g4f19118
    version, pre, git_sha = version_match.group('version', 'pre', 'git_sha')

    # When we're on a tag, `git describe --tags --candidates=50` doesn't include a commit sha.
    # We need it, so we fetch it another way.
    if not git_sha:
        # The git sha shown by `git describe --tags --candidates=50` is the first 7 characters of the sha,
        # therefore we keep the same number of characters.
        git_sha = get_commit_sha(ctx)[:7]

    pipeline_id = os.getenv("CI_PIPELINE_ID", None)

    return version, pre, commit_number, git_sha, pipeline_id


def get_matching_pattern(ctx, major_version, release=False):
    """
    We need to used specific patterns (official release tags) for nightly builds as they are used to install agent versions.
    """
    pattern = rf"{major_version}\.*"
    if release or os.getenv("BUCKET_BRANCH") in ALLOWED_REPO_NIGHTLY_BRANCHES:
        pattern = ctx.run(
            rf"git tag --list --merged {get_current_branch(ctx)} | grep -E '^{major_version}\.[0-9]+\.[0-9]+(-rc.*|-devel.*)?$' | sort -rV | head -1",
            hide=True,
        ).stdout.strip()
    return pattern
