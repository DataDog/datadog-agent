"""
Helper for running fuzz targets in the internal fuzzing infrastructure.
"""

import json
import os

from invoke import task


@task
def build_and_upload_fuzz(ctx):
    """
    This builds and uploads fuzz targets to the internal fuzzing infrastructure.
    It needs to be passed the -fuzz flag in order to build the fuzz with efficient coverage guidance.
    """
    # TODO: make these configurable
    core_count = 2
    duration = 3600  # in seconds
    proc_count = 2
    fuzz_memory = 4  # in GB
    team = "chaos-platform"  # TODO: make this use the team name from the codeowners file

    api_url = "https://fuzzing-api.us1.ddbuild.io/api/v1"
    gitsha = ctx.run('git rev-parse HEAD').stdout.strip()

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

            print(f'Building {pkgname}/{func} for {gitsha}...')
            fuzz_build_cmd = f'go test . -c -fuzz={func}$ -o {build_file} -cover -tags=test'
            ctx.run(fuzz_build_cmd)

            if not os.path.exists(directory + "/" + build_file):
                print(
                    f'❌ Build file {rel}/{build_file} does not exist. Skipping... (It is likely that we are missing a tag for this specific fuzz target to be built)'
                )
                continue

            # Get presigned URL
            print(f'Getting presigned URL for {pkgname}...')
            presigned_url_cmd = (
                f'curl -X POST "{api_url}/apps/{pkgname}/builds/{gitsha}/url" -H "Authorization: Bearer {auth_header}"'
            )
            presigned_response = ctx.run(presigned_url_cmd, hide=True).stdout.strip()
            presigned_url = json.loads(presigned_response)["data"]["url"]

            print(f'Uploading {pkgname} ({func}) for {gitsha}...')
            # Upload file to presigned URL
            upload_cmd = f'curl --request PUT --upload-file {build_file} "{presigned_url}"'
            ctx.run(upload_cmd, hide=True)

            print(f'Starting fuzzer for {pkgname} ({func})...')
            # Start new fuzzer
            run_payload = {
                "app": pkgname,
                "debug": False,
                "version": gitsha,
                "core_count": core_count,
                "duration": duration,
                "type": "go-native-fuzz",
                "function": func,
                "team": team,
                "process_count": proc_count,
                "memory": fuzz_memory,
            }

            run_json_payload = json.dumps(run_payload)
            start_fuzzer_cmd = f'curl -H "Authorization: Bearer {auth_header}" -H "Content-Type: application/json" "{api_url}/apps/{pkgname}/fuzzers" -d \'{run_json_payload}\''
            response = ctx.run(start_fuzzer_cmd, hide=True).stdout.strip()
            print(f'✅ Started fuzzer for {pkgname} ({func})...')
            response_json = json.loads(response)
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
