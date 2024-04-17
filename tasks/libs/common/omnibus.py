import hashlib
import json
import os
import sys
from datetime import datetime

import requests
from release import _get_release_json_value


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
            'ARTIFACTORY_',
            'AWS_',
            'BUILDENV_',
            'CI_',
            'CHOCOLATEY_',
            'CLUSTER_AGENT_',
            'DATADOG_AGENT_',
            'DD_',
            'DEB_',
            'DESTINATION_',
            'DOCKER_',
            'E2E_TESTS_',
            'EMISSARY_',
            'EXECUTOR_',
            'FF_',
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
            'RELEASE_VERSION',
            'RPM_',
            'RUN_',
            'S3_',
            'SMP_',
            'SSH_',
            'TEST_INFRA_',
            'USE_',
            'VAULT_',
            'WINDOWS_',
        ]
        excluded_suffixes = [
            '_SHA256',
            '_VERSION',
        ]
        excluded_values = [
            "AVAILABILITY_ZONE",
            "BENCHMARKS_CI_IMAGE",
            "BUCKET_BRANCH",
            "BUNDLER_VERSION",
            "CHANGELOG_COMMIT_SHA_SSM_NAME",
            "CLANG_LLVM_VER",
            "CHANNEL",
            "CI",
            "COMPUTERNAME" "CONSUL_HTTP_ADDR",
            "DOGSTATSD_BINARIES_DIR",
            "EXPERIMENTS_EVALUATION_ADDRESS",
            "GCE_METADATA_HOST",
            "GENERAL_ARTIFACTS_CACHE_BUCKET_URL",
            "GET_SOURCES_ATTEMPTS",
            "GO_TEST_SKIP_FLAKE",
            "HOME",
            "HOSTNAME",
            "HOST_IP",
            "INSTALL_SCRIPT_API_KEY_SSM_NAME",
            "INTEGRATION_WHEELS_CACHE_BUCKET",
            "IRBRC",
            "KITCHEN_INFRASTRUCTURE_FLAKES_RETRY",
            "LESSCLOSE",
            "LESSOPEN",
            "LC_CTYPE",
            "LS_COLORS",
            "MACOS_S3_BUCKET",
            "MESSAGE",
            "OLDPWD",
            "PROCESS_S3_BUCKET",
            "PWD",
            "PYTHON_RUNTIMES",
            "RESTORE_CACHE_ATTEMPTS",
            "RUNNER_TEMP_PROJECT_DIR",
            "RUSTC_SHA256",
            "RUST_VERSION",
            "SHLVL",
            "STATIC_BINARIES_DIR",
            "STATSD_URL",
            "SYSTEM_PROBE_BINARIES_DIR",
            "TRACE_AGENT_URL",
            "USE_CACHING_PROXY_PYTHON",
            "USE_CACHING_PROXY_RUBY",
            "USE_S3_CACHING",
            "USERDOMAIN",
            "USERNAME",
            "USERPROFILE",
            "VCPKG_BLOB_SAS_URL_SSM_NAME",
            "WIN_S3_BUCKET",
            "WINGET_PAT_SSM_NAME",
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


def omnibus_compute_cache_key(ctx):
    print('Computing cache key')
    h = hashlib.sha1()
    omnibus_last_commit = ctx.run('git log -n 1 --pretty=format:%H omnibus/', hide='stdout').stdout
    h.update(str.encode(omnibus_last_commit))
    print(f'\tLast omnibus commit is {omnibus_last_commit}')
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
    if sys.platform == 'win32':
        src_dir = "C:/buildroot/datadog-agent"
        aws_cmd = "aws.cmd"
    else:
        src_dir = os.environ.get('CI_PROJECT_DIR')
        aws_cmd = "aws"
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
    dd_api_key = ctx.run(
        f'{aws_cmd} ssm get-parameter --region us-east-1 --name {os.environ["API_KEY_ORG2_SSM_NAME"]} --with-decryption --query "Parameter.Value" --out text',
        hide=True,
    ).stdout.strip()
    headers = {'Accept': 'application/json', 'Content-Type': 'application/json', 'DD-API-KEY': dd_api_key}
    r = requests.post("https://api.datadoghq.com/api/v2/series", json={'series': series}, headers=headers)
    if r.ok:
        print('Successfully sent build metrics to DataDog')
    else:
        print(f'Failed to send build metrics to DataDog: {r.status_code}')
        print(r.text)


def install_dir_for_project(project):
    if project == "agent" or project == "iot-agent":
        folder = 'datadog-agent'
    elif project == 'dogstatsd':
        folder = 'datadog-dogstatsd'
    elif project == 'agentless-scanner':
        folder = os.path.join('datadog', 'agentless-scanner')
    elif project == 'installer':
        folder = 'datadog-installer'
    else:
        raise NotImplementedError(f'Unknown project {project}')
    return os.path.join('opt', folder)
