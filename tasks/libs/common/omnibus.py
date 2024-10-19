import hashlib
import json
import os
import sys
from datetime import datetime

import requests

from tasks.release import _get_release_json_value


def _get_build_images(ctx):
    # We intentionally include both build images & their test suffixes in the pattern
    # as a test image and the merged version shouldn't share their cache
    tags = ctx.run("grep -E 'DATADOG_AGENT_.*BUILDIMAGES' .gitlab-ci.yml | cut -d ':' -f 2", hide='stdout').stdout
    return (t.strip() for t in tags.splitlines())


def _get_omnibus_commits(field):
    if 'RELEASE_VERSION' in os.environ:
        release_version = os.environ['RELEASE_VERSION']
    else:
        release_version = os.environ['RELEASE_VERSION_7']
    return _get_release_json_value(f'{release_version}::{field}')


def _get_environment_for_cache() -> dict:
    """
    Compute a hash from the environment after excluding irrelevant/insecure
    environment variables to ensure we don't omit a variable
    """

    def env_filter(item):
        key = item[0]
        excluded_prefixes = [
            'AGENT_',
            'API_KEY_',
            'APP_KEY_',
            'AWS_',
            'BAZEL_',
            'BETA_',
            'BUILDENV_',
            'CI_',
            'CHOCOLATEY_',
            'CLUSTER_AGENT_',
            'CONDUCTOR_',
            'DATADOG_AGENT_',
            'DD_',
            'DDR_',
            'DEB_',
            'DESTINATION_',
            'DOCKER_',
            'DYNAMIC_',
            'E2E_TESTS_',
            'EMISSARY_',
            'EXECUTOR_',
            'FF_',
            'GITHUB_',
            'GITLAB_',
            'GIT_',
            'JIRA_',
            'K8S_',
            'KITCHEN_',
            'KERNEL_MATRIX_TESTING_',
            'KUBERNETES_',
            'MACOS_GITHUB_',
            'OMNIBUS_',
            'POD_',
            'PROCESSOR_',
            'RC_',
            'RELEASE_VERSION',
            'RPM_',
            'RUN_',
            'RUNNER_',
            'S3_',
            'STATS_',
            'SMP_',
            'SSH_',
            'TARGET_',
            'TEST_INFRA_',
            'USE_',
            'VAULT_',
            'XPC_',
            'WINDOWS_',
        ]
        excluded_suffixes = [
            '_SHA256',
            '_VERSION',
        ]
        excluded_values = [
            "APPS",
            "ARTIFACT_DOWNLOAD_ATTEMPTS",
            "AVAILABILITY_ZONE",
            "BENCHMARKS_CI_IMAGE",
            "BUILD_HOOK",
            "BUNDLE_MIRROR__RUBYGEMS__ORG",
            "BUCKET_BRANCH",
            "CHANGELOG_COMMIT_SHA",
            "CLANG_LLVM_VER",
            "CHANNEL",
            "CHART",
            "CI",
            "CLUSTER",
            "COMPUTERNAME",
            "CONDA_PROMPT_MODIFIER",
            "CONSUL_HTTP_ADDR",
            "DATACENTERS",
            "DDR",
            "DEPLOY_AGENT",
            "DOGSTATSD_BINARIES_DIR",
            "ENVIRONMENTS",
            "EXPERIMENTS_EVALUATION_ADDRESS",
            "FILTER",
            "FORCE_DEPLOYMENT",
            "GCE_METADATA_HOST",
            "GENERAL_ARTIFACTS_CACHE_BUCKET_URL",
            "GET_SOURCES_ATTEMPTS",
            "GO_TEST_SKIP_FLAKE",
            "HELM_HOOKS_CI_IMAGE",
            "HOME",
            "HOSTNAME",
            "HOST_IP",
            "INFOPATH",
            "INSTALL_SCRIPT_API_KEY_ORG2",
            "INTEGRATION_WHEELS_CACHE_BUCKET",
            "IRBRC",
            "KITCHEN_INFRASTRUCTURE_FLAKES_RETRY",
            "LANG",
            "LESSCLOSE",
            "LESSOPEN",
            "LC_CTYPE",
            "LS_COLORS",
            "MACOS_S3_BUCKET",
            "MANPATH",
            "MESSAGE",
            "NEW_CLUSTER",
            "OLDPWD",
            "PCP_DIR",
            "PACKAGE_ARCH",
            "PIP_INDEX_URL",
            "PROCESS_S3_BUCKET",
            "PWD",
            "PROMPT",
            "RESTORE_CACHE_ATTEMPTS",
            "RUSTC_SHA256",
            "SIGN",
            "SHELL",
            "SHLVL",
            "STATIC_BINARIES_DIR",
            "STATSD_URL",
            "SYSTEM_PROBE_BINARIES_DIR",
            "TESTING_CLEANUP",
            "TIMEOUT",
            "TMPDIR",
            "TRACE_AGENT_URL",
            "USE_S3_CACHING",
            "USER",
            "USERDOMAIN",
            "USERNAME",
            "USERPROFILE",
            "VCPKG_BLOB_SAS_URL",
            "VERSION",
            "VM_ASSETS",
            "WIN_S3_BUCKET",
            "WINGET_PAT",
            "WORKFLOW",
            "_",
            "build_before",
        ]
        for p in excluded_prefixes:
            if key.startswith(p):
                return False
        for s in excluded_suffixes:
            if key.endswith(s):
                return False
        if key in excluded_values:
            return False
        return True

    return dict(filter(env_filter, sorted(os.environ.items())))


def _last_omnibus_changes(ctx):
    omnibus_invalidating_files = ['omnibus/config/', 'omnibus/lib/', 'omnibus/omnibus.rb']
    omnibus_last_commit = ctx.run(
        f'git log -n 1 --pretty=format:%H {" ".join(omnibus_invalidating_files)}', hide='stdout'
    ).stdout
    # The commit sha1 is likely to change between a PR and its merge to main
    # In order to work around this, we hash the commit diff so that the result
    # can be reproduced on different branches with different sha1
    omnibus_last_changes = ctx.run(
        f'git diff {omnibus_last_commit}~ {omnibus_last_commit} {" ".join(omnibus_invalidating_files)}', hide='stdout'
    ).stdout
    hash = hashlib.sha1()
    hash.update(str.encode(omnibus_last_changes))
    result = hash.hexdigest()
    print(f'Hash for last omnibus changes is {result}')
    return result


def omnibus_compute_cache_key(ctx):
    print('Computing cache key')
    h = hashlib.sha1()
    omnibus_last_changes = _last_omnibus_changes(ctx)
    h.update(str.encode(omnibus_last_changes))
    buildimages_hash = _get_build_images(ctx)
    for img_hash in buildimages_hash:
        h.update(str.encode(img_hash))
    omnibus_ruby_commit = _get_omnibus_commits('OMNIBUS_RUBY_VERSION')
    omnibus_software_commit = _get_omnibus_commits('OMNIBUS_SOFTWARE_VERSION')
    print(f'Omnibus ruby commit: {omnibus_ruby_commit}')
    print(f'Omnibus software commit: {omnibus_software_commit}')
    h.update(str.encode(omnibus_ruby_commit))
    h.update(str.encode(omnibus_software_commit))
    environment = _get_environment_for_cache()
    for k, v in environment.items():
        print(f'\tUsing environment variable {k} to compute cache key')
        h.update(str.encode(f'{k}={v}'))
        print(f'Current hash value: {h.hexdigest()}')
    cache_key = h.hexdigest()
    print(f'Cache key: {cache_key}')
    return cache_key


def should_retry_bundle_install(res):
    # We sometimes get a Net::HTTPNotFound error when fetching the
    # license-scout gem. This is a transient error, so we retry the bundle install
    if "Net::HTTPNotFound:" in res.stderr:
        return True
    return False


def send_build_metrics(ctx, overall_duration):
    # We only want to generate those metrics from the CI
    src_dir = os.environ.get('CI_PROJECT_DIR')
    if sys.platform == 'win32':
        if src_dir is None:
            src_dir = os.environ.get("REPO_ROOT", os.getcwd())

    job_name = os.environ.get('CI_JOB_NAME_SLUG')
    branch = os.environ.get('CI_COMMIT_REF_NAME')
    pipeline_id = os.environ.get('CI_PIPELINE_ID')
    if not job_name or not branch or not src_dir or not pipeline_id:
        print(
            '''Missing required environment variables, this is probably not a CI job.
                  skipping sending build metrics'''
        )
        return

    series = []
    timestamp = int(datetime.now().timestamp())
    with open(f'{src_dir}/omnibus/pkg/build-summary.json') as summary_json:
        j = json.load(summary_json)
        # Various software build durations are all sent as the `datadog.agent.build.duration` metric
        # with a specific tag for each software.
        for software, metrics in j['build'].items():
            series.append(
                {
                    'metric': 'datadog.agent.build.duration',
                    'points': [{'timestamp': timestamp, 'value': metrics['build_duration']}],
                    'tags': [
                        f'software:{software}',
                        f'cached:{metrics["cached"]}',
                        f'job:{job_name}',
                        f'branch:{branch}',
                        f'pipeline:{pipeline_id}',
                    ],
                    'unit': 'seconds',
                    'type': 0,
                }
            )
        # We also provide the total duration for the omnibus build as a separate metric
        series.append(
            {
                'metric': 'datadog.agent.build.total',
                'points': [{'timestamp': timestamp, 'value': overall_duration}],
                'tags': [
                    f'job:{job_name}',
                    f'branch:{branch}',
                    f'pipeline:{pipeline_id}',
                ],
                'unit': 'seconds',
                'type': 0,
            }
        )
        # Stripping might not always be enabled so we conditionally read the metric
        if "strip" in j:
            series.append(
                {
                    'metric': 'datadog.agent.build.strip',
                    'points': [{'timestamp': timestamp, 'value': j['strip']}],
                    'tags': [
                        f'job:{job_name}',
                        f'branch:{branch}',
                        f'pipeline:{pipeline_id}',
                    ],
                    'unit': 'seconds',
                    'type': 0,
                }
            )
        # And all packagers duration as another separated metric
        for packager, duration in j['packaging'].items():
            series.append(
                {
                    'metric': 'datadog.agent.package.duration',
                    'points': [{'timestamp': timestamp, 'value': duration}],
                    'tags': [
                        f'job:{job_name}',
                        f'branch:{branch}',
                        f'packager:{packager}',
                        f'pipeline:{pipeline_id}',
                    ],
                    'unit': 'seconds',
                    'type': 0,
                }
            )
    if sys.platform == 'win32':
        dd_api_key = ctx.run(
            f'aws.cmd ssm get-parameter --region us-east-1 --name {os.environ["API_KEY_ORG2"]} --with-decryption --query "Parameter.Value" --out text',
            hide=True,
        ).stdout.strip()
    else:
        dd_api_key = ctx.run(
            f'vault kv get -field=token kv/k8s/gitlab-runner/datadog-agent/{os.environ["AGENT_API_KEY_ORG2"]}',
            hide=True,
        ).stdout.strip()
    headers = {'Accept': 'application/json', 'Content-Type': 'application/json', 'DD-API-KEY': dd_api_key}
    r = requests.post("https://api.datadoghq.com/api/v2/series", json={'series': series}, headers=headers)
    if r.ok:
        print('Successfully sent build metrics to DataDog')
    else:
        print(f'Failed to send build metrics to DataDog: {r.status_code}')
        print(r.text)


def send_cache_miss_event(ctx, pipeline_id, job_name, job_id):
    if sys.platform == 'win32':
        dd_api_key = ctx.run(
            f'aws.cmd ssm get-parameter --region us-east-1 --name {os.environ["API_KEY_ORG2"]} --with-decryption --query "Parameter.Value" --out text',
            hide=True,
        ).stdout.strip()
    else:
        dd_api_key = ctx.run(
            f'vault kv get -field=token kv/k8s/gitlab-runner/datadog-agent/{os.environ["AGENT_API_KEY_ORG2"]}',
            hide=True,
        ).stdout.strip()
    headers = {'Accept': 'application/json', 'Content-Type': 'application/json', 'DD-API-KEY': dd_api_key}
    payload = {
        'title': 'omnibus cache miss',
        'text': f"Couldn't fetch cache associated with cache key for job {job_name} in pipeline #{pipeline_id}",
        'source_type_name': 'omnibus',
        'date_happened': int(datetime.now().timestamp()),
        'tags': [f'pipeline:{pipeline_id}', f'job:{job_name}', 'source:omnibus-cache', f'job-id:{job_id}'],
    }
    r = requests.post("https://api.datadoghq.com/api/v1/events", json=payload, headers=headers)
    if not r.ok:
        print('Failed to send cache miss event')
        print(r.text)


def install_dir_for_project(project):
    if project == "agent" or project == "iot-agent":
        folder = 'datadog-agent'
    elif project == 'dogstatsd':
        folder = 'datadog-dogstatsd'
    elif project == 'installer':
        folder = 'datadog-installer'
    else:
        raise NotImplementedError(f'Unknown project {project}')
    return os.path.join('opt', folder)
