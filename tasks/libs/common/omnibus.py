import hashlib
import json
import os
import sys
from datetime import datetime

import requests

from tasks.libs.common.constants import ORIGIN_CATEGORY, ORIGIN_PRODUCT, ORIGIN_SERVICE
from tasks.libs.common.utils import get_metric_origin
from tasks.libs.releasing.version import RELEASE_JSON_DEPENDENCIES
from tasks.release import _get_release_json_value

# Increase this value to force an update to the cache key, invalidating existing
# caches and forcing a rebuild
CACHE_VERSION = 2


def _get_omnibus_commits(field):
    return _get_release_json_value(f'{RELEASE_JSON_DEPENDENCIES}::{field}')


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
            'ATLASSIAN_',
            'AWS_',
            'BAZEL_',
            'BETA_',
            'BUILDENV_',
            'CI_',
            'CHOCOLATEY_',
            'CLUSTER_AGENT_',
            'CODESIGNING_CERT_',
            'CONDUCTOR_',
            'DATADOG_AGENT_',
            'DD_',
            'DDCI_',
            'DDR_',
            'DEB_',
            'DESTINATION_',
            'DOCKER_',
            'DYNAMIC_',
            'E2E_',
            'EMISSARY_',
            'EXECUTOR_',
            'FF_',
            'GITHUB_',
            'GITLAB_',
            'GIT_',
            'INSTALLER_',
            'JIRA_',
            'K8S_',
            'KEYCHAIN_',
            'KITCHEN_',
            'KERNEL_MATRIX_TESTING_',
            'KUBERNETES_',
            'MACOS_',
            'OMNIBUS_',
            'POD_',
            'PROCESSOR_',
            'PYENV_',
            'RC_',
            'RELEASE_VERSION',
            'RPM_',
            'RUN_',
            'RUNNER_',
            'S3_',
            'STATS_',
            'SMP_',
            'SSH_',
            'TAGGER_',
            'TARGET_',
            'TEST_INFRA_',
            'USE_',
            'VAULT_',
            'VALIDATE_',
            'XPC_',
            'WINDOWS_',
        ]
        excluded_suffixes = [
            '_SHA256',
            '_VERSION',
        ]
        excluded_values = [
            "APPLE_ACCOUNT",
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
            "CLICOLOR",
            "CLUSTERS",
            "CODECOV",
            "CODECOV_TOKEN",
            "COMPARE_TO_BRANCH",
            "COMPUTERNAME",
            "CONDA_PROMPT_MODIFIER",
            "CONSUL_HTTP_ADDR",
            "DATACENTERS",
            "DDCI",
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
            "GONOSUMDB",
            "GOPROXY",
            "HELM_HOOKS_CI_IMAGE",
            "HELM_HOOKS_PERIODICAL_REBUILD_CONDUCTOR_ENV",
            "HOME",
            "HOSTNAME",
            "HOST_IP",
            "INFOPATH",
            "INSTALL_SCRIPT_API_KEY_ORG2",
            "INSTANCE_TYPE",
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
            "NEW_CLUSTER_PR_SLACK_WORKFLOW_WEBHOOK",
            "NOTIFICATIONS_SLACK_CHANNEL",
            "NOTIFIER_IMAGE",
            "OLDPWD",
            "PCP_DIR",
            "PACKAGE_ARCH",
            "PIP_EXTRA_INDEX_URL",
            "PIP_INDEX_URL",
            "PIPELINE_KEY_ALIAS",
            "PROCESS_S3_BUCKET",
            "PWD",
            "PROMPT",
            "RESTORE_CACHE_ATTEMPTS",
            "RUSTC_SHA256",
            "SIGN",
            "SHELL",
            "SHLVL",
            "SLACK_AGENT",
            "STATIC_BINARIES_DIR",
            "STATSD_URL",
            "SYSTEM_PROBE_BINARIES_DIR",
            "TEAM_ID",
            "TIMEOUT",
            "TMPDIR",
            "TRACE_AGENT_URL",
            "USER",
            "USERDOMAIN",
            "USERNAME",
            "USERPROFILE",
            "VCPKG_BLOB_SAS_URL",
            "VERSION",
            "VIRTUAL_ENV",
            "VM_ASSETS",
            "WIN_S3_BUCKET",
            "WINGET_PAT",
            "WORKFLOW",
            "_",
            "_OLD_VIRTUAL_PS1",
            "__CF_USER_TEXT_ENCODING",
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


def get_dd_api_key(ctx):
    if sys.platform == 'win32':
        cmd = f'aws.exe ssm get-parameter --region us-east-1 --name {os.environ["API_KEY_ORG2"]} --with-decryption --query "Parameter.Value" --out text'
    elif sys.platform == 'darwin':
        cmd = f'vault kv get -field=token kv/aws/arn:aws:iam::486234852809:role/ci-datadog-agent/{os.environ["AGENT_API_KEY_ORG2"]}'
    else:
        cmd = f'vault kv get -field=token kv/k8s/gitlab-runner/datadog-agent/{os.environ["AGENT_API_KEY_ORG2"]}'
    return ctx.run(cmd, hide=True).stdout.strip()


def omnibus_compute_cache_key(ctx):
    print('Computing cache key')
    h = hashlib.sha1()
    omnibus_last_changes = _last_omnibus_changes(ctx)
    h.update(str.encode(omnibus_last_changes))
    h.update(str.encode(os.getenv('CI_JOB_IMAGE', 'local_build')))
    # Some values can be forced through the environment so we need to read it
    # from there first, and fallback to release.json
    release_json_values = ['OMNIBUS_RUBY_VERSION', 'INTEGRATIONS_CORE_VERSION']
    for val_key in release_json_values:
        value = os.getenv(val_key, _get_omnibus_commits(val_key))
        print(f'{val_key}: {value}')
        h.update(str.encode(value))
    environment = _get_environment_for_cache()
    for k, v in environment.items():
        print(f'\tUsing environment variable {k} to compute cache key')
        h.update(str.encode(f'{k}={v}'))
        print(f'Current hash value: {h.hexdigest()}')
    cache_key = h.hexdigest()
    cache_key += f'_{CACHE_VERSION}'
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
                    "metadata": get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE, True),
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
                "metadata": get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE, True),
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
                    "metadata": get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE, True),
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
                    "metadata": get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE, True),
                }
            )

    headers = {'Accept': 'application/json', 'Content-Type': 'application/json', 'DD-API-KEY': get_dd_api_key(ctx)}
    r = requests.post("https://api.datadoghq.com/api/v2/series", json={'series': series}, headers=headers, timeout=10)
    if r.ok:
        print('Successfully sent build metrics to DataDog')
    else:
        print(f'Failed to send build metrics to DataDog: {r.status_code}')
        print(r.text)


def send_cache_mutation_event(ctx, pipeline_id, job_name, job_id):
    headers = {'Accept': 'application/json', 'Content-Type': 'application/json', 'DD-API-KEY': get_dd_api_key(ctx)}
    payload = {
        'title': 'omnibus cache mutated',
        'text': f"Job {job_name} in pipeline #{pipeline_id} attempted to mutate the cache after a hit",
        'source_type_name': 'omnibus',
        'date_happened': int(datetime.now().timestamp()),
        'tags': [f'pipeline:{pipeline_id}', f'job:{job_name}', 'source:omnibus-cache', f'job-id:{job_id}'],
    }
    r = requests.post("https://api.datadoghq.com/api/v1/events", json=payload, headers=headers)
    if not r.ok:
        print('Failed to send cache mutation event')
        print(r.text)


def send_cache_miss_event(ctx, pipeline_id, job_name, job_id):
    headers = {'Accept': 'application/json', 'Content-Type': 'application/json', 'DD-API-KEY': get_dd_api_key(ctx)}
    payload = {
        'title': 'omnibus cache miss',
        'text': f"Couldn't fetch cache associated with cache key for job {job_name} in pipeline #{pipeline_id}",
        'source_type_name': 'omnibus',
        'date_happened': int(datetime.now().timestamp()),
        'tags': [f'pipeline:{pipeline_id}', f'job:{job_name}', 'source:omnibus-cache', f'job-id:{job_id}'],
    }
    r = requests.post("https://api.datadoghq.com/api/v1/events", json=payload, headers=headers, timeout=10)
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
