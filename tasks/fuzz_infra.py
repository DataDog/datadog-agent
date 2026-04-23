"""
Helper for running fuzz targets in the internal fuzzing infrastructure.
"""

import os
import sys

import requests
from invoke import task

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_commit_sha
from tasks.libs.owners.parsing import search_owners
from tasks.libs.pipeline.notifications import GITHUB_SLACK_MAP

DEFAULT_FUZZING_SLACK_CHANNEL = "agent-fuzz-findings"
GO_BUILD_TAGS = "test,linux_bpf,nvml,amd64,zlib,zstd"


def get_slack_channel_for_directory(directory_path: str) -> str:
    """
    Get the Slack channel associated with a directory based on CODEOWNERS.
    If multiple teams are possible, take the first one.

    Args:
        directory_path: The directory path to find owners for

    Returns:
        The Slack channel for the first owner, or default channel if no owner found
    """
    try:
        # Assert that the path is either relative or had the expected prefix
        assert (
            not directory_path.startswith('/') or directory_path.startswith("/go/src/github.com/DataDog/datadog-agent/")
        ), f"Expected relative path or path starting with '/go/src/github.com/DataDog/datadog-agent/', got: {directory_path}"

        # Remove the leading datadog-agent prefix if it exists
        rel_path = directory_path.removeprefix("/go/src/github.com/DataDog/datadog-agent/")

        # Search for owners of this path
        owners = search_owners(rel_path, ".github/CODEOWNERS")

        if not owners:
            return DEFAULT_FUZZING_SLACK_CHANNEL

        # Take the first owner, we assume the first one is enough.
        # The api currently only supports one slack channel per fuzz target. If need be we could change this.
        first_owner = owners[0].lower()

        # Map the owner to a slack channel
        return GITHUB_SLACK_MAP.get(first_owner, DEFAULT_FUZZING_SLACK_CHANNEL).replace("#", "")

    except Exception as e:
        print(
            f"{color_message('Warning', Color.ORANGE)}: Could not determine slack channel for {directory_path}: {e}",
            file=sys.stderr,
        )
        return DEFAULT_FUZZING_SLACK_CHANNEL


@task
def build_and_upload_fuzz(
    ctx, team="chaos-platform", core_count=None, duration=None, proc_count=None, fuzz_memory=None
):
    """
    This builds and uploads fuzz targets to the internal fuzzing infrastructure.
    It needs to be passed the -fuzz flag in order to build the fuzz with efficient coverage guidance.
    """

    api_url = "https://fuzzing-api.us1.ddbuild.io/api/v1"
    git_sha = get_commit_sha(ctx)

    # Get the auth token a single time and reuse it for all requests
    auth_header = ctx.run(
        'vault read -field=token identity/oidc/token/security-fuzzing-platform', hide=True
    ).stdout.strip()

    max_pkg_name_length = 50
    for directory, func in search_fuzz_tests(os.getcwd()):
        with ctx.cd(directory):
            # eg: convert "/path/to/fuzz/target" to "datadog-agent-path-to-fuzz-target".
            # It's a unique identifier for the fuzz target.
            # We also append the function name to the package name to make sure that every function inside the same package
            # has a unique target name. This allows us to have different inputs for different functions in the same package.
            rel = directory.removeprefix("/go/src/github.com/DataDog/datadog-agent/")
            pkgname = "datadog-agent-"
            pkgname += "-".join(rel.split('/'))[:max_pkg_name_length]
            pkgname += f"-{func}"
            build_file = "fuzz.test"

            print(f'Building {pkgname}/{func} for {git_sha}...')
            fuzz_build_cmd = f'go test . -c -fuzz={func}$ -o {build_file} -cover -tags={GO_BUILD_TAGS}'
            try:
                ctx.run(fuzz_build_cmd)
            except Exception as e:
                print(f'❌ Failed to build {pkgname}: {e}... Skipping this fuzz target')
                continue

            build_full_path = directory + "/" + build_file
            if not os.path.exists(build_full_path):
                print(
                    f'❌ Build file {rel}/{build_file} does not exist. Skipping... (It is likely that we are missing a tag for this specific fuzz target to be built)'
                )
                continue

            # Get presigned URL
            print(f'Getting presigned URL for {pkgname}...')
            headers = {"Authorization": f"Bearer {auth_header}"}
            presigned_response = requests.post(
                f"{api_url}/apps/{pkgname}/builds/{git_sha}/url", headers=headers, timeout=30
            )
            presigned_response.raise_for_status()
            presigned_url = presigned_response.json()["data"]["url"]

            print(f'Uploading {pkgname} ({func}) for {git_sha}...')
            # Upload file to presigned URL
            with open(build_full_path, 'rb') as f:
                upload_response = requests.put(presigned_url, data=f, timeout=300)
                upload_response.raise_for_status()

            print(f'Starting fuzzer for {pkgname} ({func})...')
            # Start new fuzzer
            run_payload = {
                "app": pkgname,  # required
                "version": git_sha,  # required
                "type": "go-native-fuzz",  # required
                "function": func,  # required
                "team": team,  # Optional, but in this repository we always want to set it.
                "slack_channel": get_slack_channel_for_directory(
                    directory
                ),  # Optional, but in this repository we always want to set it as we have an up to date mapping.
            }

            # Optional parameters where we want the backend to set the values.
            if core_count:
                run_payload["core_count"] = core_count
            if duration:
                run_payload["duration"] = duration
            if fuzz_memory:
                run_payload["memory"] = fuzz_memory
            if proc_count:
                run_payload["process_count"] = proc_count

            headers = {"Authorization": f"Bearer {auth_header}", "Content-Type": "application/json"}
            response = requests.post(f"{api_url}/apps/{pkgname}/fuzzers", headers=headers, json=run_payload, timeout=30)
            response.raise_for_status()
            print(f'✅ Started fuzzer for {pkgname} ({func})...')
            response_json = response.json()
            print(response_json)


def search_fuzz_tests(directory):
    """
    Yields (directory, fuzz function name) tuples.
    """
    for file in os.listdir(directory):
        path = os.path.join(directory, file)
        if os.path.isdir(path):
            # Skip hidden directories (.cache, .git, etc.) to avoid picking up
            # files from the bazel cache or other non-source directories.
            if file.startswith('.'):
                continue
            yield from search_fuzz_tests(path)
        else:
            if not file.endswith('_test.go'):
                continue
            with open(path) as f:
                for line in f.readlines():
                    # we only want to support Go native fuzzing for now. So it must contain `*testing.F` argument
                    if line.startswith('func Fuzz') and '*testing.F' in line:
                        fuzzfunc = line[5 : line.find('(')]  # 5 is len('func ')
                        yield directory, fuzzfunc
