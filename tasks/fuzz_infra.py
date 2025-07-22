"""
Helper for running fuzz targets in the internal fuzzing infrastructure.
"""

import os

import requests
from invoke import task

from tasks.libs.common.git import get_commit_sha


@task
def build_and_upload_fuzz(ctx, team="chaos-platform", core_count=2, duration=3600, proc_count=2, fuzz_memory=4):
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
            rel = directory.removeprefix("/go/src/github.com/DataDog/datadog-agent/")
            pkgname = "datadog-agent-"
            pkgname += "-".join(rel.split('/'))[:max_pkg_name_length]
            build_file = "fuzz.test"

            print(f'Building {pkgname}/{func} for {git_sha}...')
            fuzz_build_cmd = f'go test . -c -fuzz={func}$ -o {build_file} -cover -tags=test'
            ctx.run(fuzz_build_cmd)
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
                "app": pkgname,
                "debug": False,
                "version": git_sha,
                "core_count": core_count,
                "duration": duration,
                "type": "go-native-fuzz",
                "function": func,
                "team": team,
                "process_count": proc_count,
                "memory": fuzz_memory,
            }

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
