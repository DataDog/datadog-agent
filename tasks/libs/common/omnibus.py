import hashlib
import json
import os
import sys
from datetime import datetime

import requests

from tasks.libs.common.constants import ORIGIN_CATEGORY, ORIGIN_PRODUCT, ORIGIN_SERVICE
from tasks.libs.common.utils import get_metric_origin

# Increase this value to force an update to the cache key, invalidating existing
# caches and forcing a rebuild
CACHE_VERSION = 2


ENV_PASSHTROUGH = {
    'BAZELISK_HOME': "Runner-dependent cache path used by `bazelisk` to manage `bazel` installations",
    'CI': "dda and `bazel` rely on this to be able to tell whether they're running on CI and adapt behavior",
    'DD_CC': 'Points at c compiler',
    'DD_CXX': 'Points at c++ compiler',
    'SKIP_PKG_COMPRESSION': 'Skip package XZ compression (set to true for faster local builds)',
    'DD_CMAKE_TOOLCHAIN': 'Points at cmake toolchain',
    'DDA_NO_DYNAMIC_DEPS': 'Variable affecting dda behavior',
    'E2E_COVERAGE_PIPELINE': 'Used to do a special build of the agent to generate coverage data',
    'DEPLOY_AGENT': 'Used to apply higher compression level for deployed artifacts',
    'FORCED_PACKAGE_COMPRESSION_LEVEL': 'Used as an override for the compression level of artifacts',
    'GEM_HOME': 'rvm / Ruby stuff to make sure Omnibus itself runs correctly',
    'GEM_PATH': 'rvm / Ruby stuff to make sure Omnibus itself runs correctly',
    'HOME': 'Home directory might be used by invoked programs such as git',
    'INSTALL_DIR': 'Used by Omnibus to determine the target install directory when building the package',
    'INTEGRATION_WHEELS_CACHE_BUCKET': 'Bucket where integration wheels are cached',
    'INTEGRATION_WHEELS_SKIP_CACHE_UPLOAD': 'Setting that skips uploading integration wheels to cache',
    'MY_RUBY_HOME': 'rvm / Ruby stuff to make sure Omnibus itself runs correctly',
    'OMNIBUS_FORCE_PACKAGES': 'Force Omnibus to build actual packages',
    'OMNIBUS_GIT_CACHE_DIR': 'Local directory used by Omnibus for the local git cache',
    'OMNIBUS_PACKAGE_ARTIFACT_DIR': 'Directory to take the base artifact from on "packaging-only" mode',
    'PACKAGE_ARCH': 'Target architecture',
    'PATH': 'Needed to find binaries',
    'PKG_CONFIG_LIBDIR': 'pkgconfig variable',
    'PKG_CONFIG_PATH': 'pkgconfig variable',
    'PYTHONUTF8': 'Hint for Python to use UTF-8',
    'RUBY_VERSION': 'Used by Omnibus / Gemspec',
    'S3_OMNIBUS_CACHE_ANONYMOUS_ACCESS': 'Use to determine whether Omnibus can write to the artifact cache',
    'S3_OMNIBUS_CACHE_BUCKET': 'Points at bucket used for Omnibus source artifacts',
    'SSH_AUTH_SOCK': 'Developer environments configure Git to use SSH authentication',
    'XDG_CACHE_HOME': "Runner-dependent cache path used by `bazel` (natively on POSIX OSes, emulated on Windows)",
    'rvm_path': 'rvm / Ruby stuff to make sure Omnibus itself runs correctly',
    'rvm_bin_path': 'rvm / Ruby stuff to make sure Omnibus itself runs correctly',
    'rvm_prefix': 'rvm / Ruby stuff to make sure Omnibus itself runs correctly',
    'rvm_version': 'rvm / Ruby stuff to make sure Omnibus itself runs correctly',
    'AGENT_DATA_PLANE_VERSION': 'Agent Data Plane Version',
}

OS_SPECIFIC_ENV_PASSTHROUGH = {
    'win32': {
        'APPDATA': 'Windows-specific folder',
        'JARSIGN_JAR': 'Used for signing MSI packages',
        'LOCALAPPDATA': 'Required for go build cache (and maybe more)',
        'MSYSTEM': 'MSYS2-related',
        'MINGW_PACKAGE_PREFIX': 'MSYS2-related',
        'PATHEXT': 'Extensions that Windows considers as executable',
        'PROCESSOR_ARCHITECTURE': 'Needed for architecture detection',
        'PROGRAMFILES': 'Standard Windows installation location',
        'PROGRAMFILES(X86)': 'Standard Windows installation location',
        'PROGRAMFILESW6432': 'Standard Windows installation location',
        'SIGN_WINDOWS_DD_WCS': 'Determines whether to sign Windows artifacts',
        'SSL_CERT_FILE': 'Used to point Ruby at the certificate for OpenSSL',
        'SYSTEMDRIVE': "goes with SYSTEMROOT",
        'SYSTEMROOT': 'Solves git: fatal: getaddrinfo() thread failed to start',
        'TEMP': 'Temporary directory',
        'TMP': 'Temporary directory',
        'USERPROFILE': 'Home directory for Windows',
        'VCINSTALLDIR': 'For symbol inspector',
        'VSTUDIO_ROOT': 'For symbol inspector',
        'WINDIR': 'Windows operating system directory',
        'WINDOWS_BUILDER': 'Used to decide whether to assume a role for S3 access',
        'WINDOWS_DDNPM_DRIVER': 'Windows Network Driver',
        'WINDOWS_DDNPM_VERSION': 'Windows Network Driver Version',
        'WINDOWS_DDNPM_SHASUM': 'Windows Network Driver Checksum',
        'WINDOWS_DDPROCMON_DRIVER': 'Windows Kernel Procmon Driver',
        'WINDOWS_DDPROCMON_VERSION': 'Windows Kernel Procmon Driver Version',
        'WINDOWS_DDPROCMON_SHASUM': 'Windows Kernel Procmon Driver Checksum',
    },
    'linux': {
        'DEB_GPG_KEY': 'Used to sign packages',
        'DEB_GPG_KEY_NAME': 'Used to sign packages',
        'DEB_SIGNING_PASSPHRASE': 'Used to sign packages',
        'LD_PRELOAD': 'Needed to fake armv7l architecture (via libfakearmv7l.so, see Dockerfile for rpm_armhf buildimage)',
        'RPM_GPG_KEY': 'Used to sign packages',
        'RPM_GPG_KEY_NAME': 'Used to sign packages',
        'RPM_SIGNING_PASSPHRASE': 'Used to sign packages',
        'AGENT_DATA_PLANE_HASH_LINUX_AMD64': 'Agent Data Plane Hash for Linux AMD64',
        'AGENT_DATA_PLANE_HASH_LINUX_ARM64': 'Agent Data Plane Hash for Linux ARM64',
        'AGENT_DATA_PLANE_HASH_FIPS_LINUX_AMD64': 'Agent Data Plane Hash for FIPS Linux AMD64',
        'AGENT_DATA_PLANE_HASH_FIPS_LINUX_ARM64': 'Agent Data Plane Hash for FIPS Linux ARM64',
    },
    'darwin': {},
}


def _get_environment_for_cache(env: dict[str, str]) -> dict:
    """
    Compute a hash from the environment after excluding irrelevant/insecure
    environment variables to ensure we don't omit a variable
    """
    excluded_variables = {
        'APPDATA',
        'DEB_GPG_KEY',
        'DEB_GPG_KEY_NAME',
        'DEB_SIGNING_PASSPHRASE',
        'GEM_HOME',
        'GEM_PATH',
        'HOME',
        'JARSIGN_JAR',
        'LD_PRELOAD',
        'LOCALAPPDATA',
        'MY_RUBY_HOME',
        'OMNIBUS_GIT_CACHE_DIR',
        'OMNIBUS_GOMODCACHE',
        'OMNIBUS_WORKERS_OVERRIDE',
        'PACKAGE_VERSION',
        'PYTHONUTF8',
        'RPM_GPG_KEY',
        'RPM_GPG_KEY_NAME',
        'RPM_SIGNING_PASSPHRASE',
        'S3_OMNIBUS_CACHE_ANONYMOUS_ACCESS',
        'SIGN_WINDOWS_DD_WCS',
        'SSH_AUTH_SOCK',
        'SYSTEMDRIVE',
        'SYSTEMROOT',
        'SSL_CERT_FILE',
        'TEMP',
        'TMP',
        'USERPROFILE',
        'rvm_bin_path',
        'rvm_path',
        'rvm_prefix',
        'rvm_version',
    }
    return {k: v for k, v in env.items() if k not in excluded_variables}


def _hash_paths(hasher, paths: list[str]):
    """Update hashlib.hash object `hasher` by recursive hashing of the contents provided in `paths`."""

    def hash_file(filepath):
        # Include the path to the file in the hash to account for file moves
        hasher.update(filepath.encode())

        # Hash the file contents
        with open(filepath, 'rb') as f:
            while chunk := f.read(4096):
                hasher.update(chunk)

    def all_files_under(path):
        for root, _, filenames in os.walk(path):
            for filename in filenames:
                yield os.path.join(root, filename)

    for path in sorted(paths):
        if os.path.isfile(path):
            hash_file(path)
        elif os.path.isdir(path):
            for filepath in sorted(all_files_under(path)):
                hash_file(filepath)
        else:
            raise ValueError("provided paths must exist and be either a folder or a regular file")


def get_dd_api_key(ctx):
    if sys.platform == 'win32':
        cmd = f'aws.exe ssm get-parameter --region us-east-1 --name {os.environ["API_KEY_ORG2"]} --with-decryption --query "Parameter.Value" --out text'
    elif sys.platform == 'darwin':
        cmd = f'vault kv get -field=token kv/aws/arn:aws:iam::486234852809:role/ci-datadog-agent/{os.environ["AGENT_API_KEY_ORG2"]}'
    else:
        cmd = f'vault kv get -field=token kv/k8s/{os.environ["POD_NAMESPACE"]}/datadog-agent/{os.environ["AGENT_API_KEY_ORG2"]}'
    return ctx.run(cmd, hide=True).stdout.strip()


def omnibus_compute_cache_key(ctx, env: dict[str, str]) -> str:
    print('Computing cache key')
    h = hashlib.sha1()
    _hash_paths(
        h,
        [
            'omnibus/config',
            'omnibus/lib',
            'omnibus/package-scripts',
            'omnibus/python-scripts',
            'omnibus/resources',
            'omnibus/omnibus.rb',
            'deps',
            'bazel',
        ],
    )
    print(f'Current hash value: {h.hexdigest()}')
    h.update(str.encode(os.getenv('CI_JOB_IMAGE', 'local_build')))
    environment = _get_environment_for_cache(env)
    for k, v in sorted(environment.items()):
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
