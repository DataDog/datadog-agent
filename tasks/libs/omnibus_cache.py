import hashlib
import os


def _get_build_images(ctx):
    # We intentionally include both build images & their test suffixes in the pattern
    # as a test image and the merged version shouldn't share their cache
    tags = ctx.run("grep -E 'DATADOG_AGENT_.*BUILDIMAGES' .gitlab-ci.yml | cut -d ':' -f 2", hide='stdout').stdout
    return (t.strip() for t in tags.splitlines())


def _get_environment_for_cache() -> dict:
    """
    Compute a hash from the environment after excluding irrelevant/insecure
    environment variables to ensure we don't omit a variable
    """

    def env_filter(item):
        key = item[0]
        excluded_prefixes = [
            'AGENT_',
            'ARTIFACTORY_',
            'AWS_',
            'BUILDENV_',
            'CI_',
            'CLUSTER_AGENT_',
            'DATADOG_AGENT_',
            'DD_',
            'DEB_',
            'DESTINATION_',
            'DOCKER_',
            'FF_',
            'GITLAB_',
            'GIT_',
            'K8S_',
            'KERNEL_MATRIX_TESTING_',
            'KUBERNETES_',
            'OMNIBUS_',
            'POD_',
            'RELEASE_VERSION',
            'RPM_',
            'S3_',
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
            "CHANNEL",
            "CI",
            "CONSUL_HTTP_ADDR",
            "DOGSTATSD_BINARIES_DIR",
            "EXPERIMENTS_EVALUATION_ADDRESS",
            "GCE_METADATA_HOST",
            "GENERAL_ARTIFACTS_CACHE_BUCKET_URL",
            "GET_SOURCES_ATTEMPTS",
            "HOME",
            "HOSTNAME",
            "HOST_IP",
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
            "RUN_ALL_BUILDS",
            "RUN_KITCHEN_TESTS",
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
            "WIN_S3_BUCKET",
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
    environment = _get_environment_for_cache()
    for k, v in environment.items():
        print(f'\tUsing environment variable {k} to compute cache key')
        h.update(str.encode(f'{k}={v}'))
    # FIXME: include omnibus-ruby and omnibus-software version once they are pinned
    cache_key = h.hexdigest()
    print(f'Cache key: {cache_key}')
    return cache_key
