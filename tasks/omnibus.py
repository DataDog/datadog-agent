import os
import sys

from invoke import task

from tasks.flavor import AgentFlavor
from tasks.go import deps
from tasks.libs.common.omnibus import (
    install_dir_for_project,
    omnibus_compute_cache_key,
    send_build_metrics,
    should_retry_bundle_install,
)
from tasks.libs.common.utils import get_version, load_release_versions, timed
from tasks.ssm import get_pfx_pass, get_signing_cert


def omnibus_run_task(
    ctx, task, target_project, base_dir, env, omnibus_s3_cache=False, log_level="info", host_distribution=None
):
    with ctx.cd("omnibus"):
        overrides_cmd = ""
        if base_dir:
            overrides_cmd = f"--override=base_dir:{base_dir}"
        if host_distribution:
            overrides_cmd += f" --override=host_distribution:{host_distribution}"

        omnibus = "bundle exec omnibus"
        if sys.platform == 'win32':
            omnibus = "bundle exec omnibus.bat"
        elif sys.platform == 'darwin':
            # HACK: This is an ugly hack to fix another hack made by python3 on MacOS
            # The full explanation is available on this PR: https://github.com/DataDog/datadog-agent/pull/5010.
            omnibus = "unset __PYVENV_LAUNCHER__ && bundle exec omnibus"

        if omnibus_s3_cache:
            populate_s3_cache = "--populate-s3-cache"
        else:
            populate_s3_cache = ""

        cmd = "{omnibus} {task} {project_name} --log-level={log_level} {populate_s3_cache} {overrides}"
        args = {
            "omnibus": omnibus,
            "task": task,
            "project_name": target_project,
            "log_level": log_level,
            "overrides": overrides_cmd,
            "populate_s3_cache": populate_s3_cache,
        }

        ctx.run(cmd.format(**args), env=env)


def bundle_install_omnibus(ctx, gem_path=None, env=None, max_try=2):
    with ctx.cd("omnibus"):
        # make sure bundle install starts from a clean state
        try:
            os.remove("Gemfile.lock")
        except Exception:
            pass

        cmd = "bundle install"
        if gem_path:
            cmd += f" --path {gem_path}"

        for trial in range(max_try):
            res = ctx.run(cmd, env=env, warn=True)
            if res.ok:
                return
            if not should_retry_bundle_install(res):
                return
            print(f"Retrying bundle install, attempt {trial + 1}/{max_try}")


def get_omnibus_env(
    ctx,
    skip_sign=False,
    release_version="nightly",
    major_version='7',
    python_runtimes='3',
    hardened_runtime=False,
    system_probe_bin=None,
    go_mod_cache=None,
    flavor=AgentFlavor.base,
    pip_config_file="pip.conf",
):
    env = load_release_versions(ctx, release_version)

    # If the host has a GOMODCACHE set, try to reuse it
    if not go_mod_cache and os.environ.get('GOMODCACHE'):
        go_mod_cache = os.environ.get('GOMODCACHE')

    if go_mod_cache:
        env['OMNIBUS_GOMODCACHE'] = go_mod_cache

    if int(major_version) > 6:
        env['OMNIBUS_OPENSSL_SOFTWARE'] = 'openssl3'

    env_override = ['INTEGRATIONS_CORE_VERSION', 'OMNIBUS_SOFTWARE_VERSION']
    for key in env_override:
        value = os.environ.get(key)
        # Only overrides the env var if the value is a non-empty string.
        if value:
            env[key] = value

    if sys.platform == 'win32' and os.environ.get('SIGN_WINDOWS'):
        # get certificate and password from ssm
        pfxfile = get_signing_cert(ctx)
        pfxpass = get_pfx_pass(ctx)
        env['SIGN_PFX'] = str(pfxfile)
        env['SIGN_PFX_PW'] = str(pfxpass)

    if sys.platform == 'darwin':
        # Target MacOS 10.12
        env['MACOSX_DEPLOYMENT_TARGET'] = '10.12'

    if skip_sign:
        env['SKIP_SIGN_MAC'] = 'true'
    if hardened_runtime:
        env['HARDENED_RUNTIME_MAC'] = 'true'

    env['PACKAGE_VERSION'] = get_version(
        ctx, include_git=True, url_safe=True, major_version=major_version, include_pipeline_id=True
    )
    env['MAJOR_VERSION'] = major_version
    env['PY_RUNTIMES'] = python_runtimes

    # Since omnibus and the invoke task won't run in the same folder
    # we need to input the absolute path of the pip config file
    env['PIP_CONFIG_FILE'] = os.path.abspath(pip_config_file)

    if system_probe_bin:
        env['SYSTEM_PROBE_BIN'] = system_probe_bin
    env['AGENT_FLAVOR'] = flavor.name

    # We need to override the workers variable in omnibus build when running on Kubernetes runners,
    # otherwise, ohai detect the number of CPU on the host and run the make jobs with all the CPU.
    if os.environ.get('KUBERNETES_CPU_REQUEST'):
        env['OMNIBUS_WORKERS_OVERRIDE'] = str(int(os.environ.get('KUBERNETES_CPU_REQUEST')) + 1)
    # Forward the DEPLOY_AGENT variable so that we can use a higher compression level for deployed artifacts
    if os.environ.get('DEPLOY_AGENT'):
        env['DEPLOY_AGENT'] = os.environ.get('DEPLOY_AGENT')
    if 'PACKAGE_ARCH' in os.environ:
        env['PACKAGE_ARCH'] = os.environ.get('PACKAGE_ARCH')
    if 'INSTALL_DIR' in os.environ:
        print('Forwarding INSTALL_DIR')
        env['INSTALL_DIR'] = os.environ.get('INSTALL_DIR')

    return env


# hardened-runtime needs to be set to False to build on MacOS < 10.13.6, as the -o runtime option is not supported.
@task(
    help={
        'skip-sign': "On macOS, use this option to build an unsigned package if you don't have Datadog's developer keys.",
        'hardened-runtime': "On macOS, use this option to enforce the hardened runtime setting, adding '-o runtime' to all codesign commands",
    }
)
def build(
    ctx,
    flavor=AgentFlavor.base.name,
    agent_binaries=False,
    log_level="info",
    base_dir=None,
    gem_path=None,
    skip_deps=False,
    skip_sign=False,
    release_version="nightly",
    major_version='7',
    python_runtimes='3',
    omnibus_s3_cache=False,
    hardened_runtime=False,
    system_probe_bin=None,
    go_mod_cache=None,
    python_mirror=None,
    pip_config_file="pip.conf",
    host_distribution=None,
    install_directory=None,
    target_project=None,
):
    """
    Build the Agent packages with Omnibus Installer.
    """

    flavor = AgentFlavor[flavor]
    if not skip_deps:
        with timed(quiet=True) as deps_elapsed:
            deps(ctx)

    # base dir (can be overridden through env vars, command line takes precedence)
    base_dir = base_dir or os.environ.get("OMNIBUS_BASE_DIR")

    if base_dir is not None and sys.platform == 'win32':
        # On Windows, prevent backslashes in the base_dir path otherwise omnibus will fail with
        # error 'no matched files for glob copy' at the end of the build.
        base_dir = base_dir.replace(os.path.sep, '/')

    env = get_omnibus_env(
        ctx,
        skip_sign=skip_sign,
        release_version=release_version,
        major_version=major_version,
        python_runtimes=python_runtimes,
        hardened_runtime=hardened_runtime,
        system_probe_bin=system_probe_bin,
        go_mod_cache=go_mod_cache,
        flavor=flavor,
        pip_config_file=pip_config_file,
    )

    if not target_project:
        target_project = "agent"
        if flavor.is_iot():
            target_project = "iot-agent"
        elif agent_binaries:
            target_project = "agent-binaries"

    aws_cmd = "aws.cmd" if sys.platform == 'win32' else "aws"

    # Get the python_mirror from the PIP_INDEX_URL environment variable if it is not passed in the args
    python_mirror = python_mirror or os.environ.get("PIP_INDEX_URL")

    # If a python_mirror is set then use it for pip by adding it in the pip.conf file
    pip_index_url = f"[global]\nindex-url = {python_mirror}" if python_mirror else ""

    # We're passing the --index-url arg through a pip.conf file so that omnibus doesn't leak the token
    with open(pip_config_file, 'w') as f:
        f.write(pip_index_url)

    with timed(quiet=True) as bundle_elapsed:
        bundle_install_omnibus(ctx, gem_path, env)

    omnibus_cache_dir = os.environ.get('OMNIBUS_GIT_CACHE_DIR')
    use_omnibus_git_cache = omnibus_cache_dir is not None and target_project == "agent"
    if use_omnibus_git_cache:
        # The cache will be written in the provided cache dir (see omnibus.rb) but
        # the git repository itself will be located in a subfolder that replicates
        # the install_dir hierarchy
        # For instance if git_cache_dir is set to "/git/cache/dir" and install_dir is
        # set to /a/b/c, the cache git repository will be located in
        # /git/cache/dir/a/b/c/.git
        if install_directory is None:
            install_directory = install_dir_for_project(target_project)
        # Is the path starts with a /, it's considered the new root for the joined path
        # which effectively drops whatever was in omnibus_cache_dir
        install_directory = install_directory.lstrip('/')
        omnibus_cache_dir = os.path.join(omnibus_cache_dir, install_directory)
        remote_cache_name = os.environ.get('CI_JOB_NAME_SLUG')
        # We don't want to update the cache when not running on a CI
        # Individual developers are still able to leverage the cache by providing
        # the OMNIBUS_GIT_CACHE_DIR env variable, but they won't pull from the CI
        # generated one.
        use_remote_cache = remote_cache_name is not None
        if use_remote_cache:
            cache_state = None
            cache_key = omnibus_compute_cache_key(ctx)
            git_cache_url = f"s3://{os.environ['S3_OMNIBUS_CACHE_BUCKET']}/builds/{cache_key}/{remote_cache_name}"
            bundle_path = (
                "/tmp/omnibus-git-cache-bundle" if sys.platform != 'win32' else "C:\\TEMP\\omnibus-git-cache-bundle"
            )
            with timed(quiet=True) as restore_cache:
                # Allow failure in case the cache was evicted
                if ctx.run(f"{aws_cmd} s3 cp --only-show-errors {git_cache_url} {bundle_path}", warn=True):
                    print(f'Successfully restored cache {cache_key}')
                    ctx.run(f"git clone --mirror {bundle_path} {omnibus_cache_dir}")
                    cache_state = ctx.run(f"git -C {omnibus_cache_dir} tag -l").stdout
                else:
                    print(f'Failed to restore cache from key {cache_key}')

    with timed(quiet=True) as omnibus_elapsed:
        omnibus_run_task(
            ctx=ctx,
            task="build",
            target_project=target_project,
            base_dir=base_dir,
            env=env,
            omnibus_s3_cache=omnibus_s3_cache,
            log_level=log_level,
            host_distribution=host_distribution,
        )

    # Delete the temporary pip.conf file once the build is done
    os.remove(pip_config_file)

    if use_omnibus_git_cache:
        stale_tags = ctx.run(f'git -C {omnibus_cache_dir} tag --no-merged', warn=True).stdout
        # Purge the cache manually as omnibus will stick to not restoring a tag when
        # a mismatch is detected, but will keep the old cached tags.
        # Do this before checking for tag differences, in order to remove staled tags
        # in case they were included in the bundle in a previous build
        for _, tag in enumerate(stale_tags.split(os.linesep)):
            ctx.run(f'git -C {omnibus_cache_dir} tag -d {tag}')
        if use_remote_cache and ctx.run(f"git -C {omnibus_cache_dir} tag -l").stdout != cache_state:
            with timed(quiet=True) as update_cache:
                ctx.run(f"git -C {omnibus_cache_dir} bundle create {bundle_path} --tags")
                ctx.run(f"{aws_cmd} s3 cp --only-show-errors {bundle_path} {git_cache_url}")

    print("Build component timing:")
    if not skip_deps:
        print(f"Deps:    {deps_elapsed.duration}")
    print(f"Bundle:  {bundle_elapsed.duration}")
    print(f"Omnibus: {omnibus_elapsed.duration}")
    if use_omnibus_git_cache and use_remote_cache:
        print(f"Restoring omnibus cache: {restore_cache.duration}")
        print(f"Updating omnibus cache: {update_cache.duration}")
    send_build_metrics(ctx, omnibus_elapsed.duration)


@task
def manifest(
    ctx,
    platform=None,
    arch=None,
    flavor=AgentFlavor.base.name,
    agent_binaries=False,
    log_level="info",
    base_dir=None,
    gem_path=None,
    skip_sign=False,
    release_version="nightly",
    major_version='7',
    python_runtimes='3',
    hardened_runtime=False,
    system_probe_bin=None,
    go_mod_cache=None,
):
    flavor = AgentFlavor[flavor]
    # base dir (can be overridden through env vars, command line takes precedence)
    base_dir = base_dir or os.environ.get("OMNIBUS_BASE_DIR")

    env = get_omnibus_env(
        ctx,
        skip_sign=skip_sign,
        release_version=release_version,
        major_version=major_version,
        python_runtimes=python_runtimes,
        hardened_runtime=hardened_runtime,
        system_probe_bin=system_probe_bin,
        go_mod_cache=go_mod_cache,
        flavor=flavor,
    )

    target_project = "agent"
    if flavor.is_iot():
        target_project = "iot-agent"
    elif agent_binaries:
        target_project = "agent-binaries"

    bundle_install_omnibus(ctx, gem_path, env)

    task = "manifest"
    if platform is not None:
        task += f" --platform-family={platform} --platform={platform} "
    if arch is not None:
        task += f" --architecture={arch} "

    omnibus_run_task(
        ctx=ctx,
        task=task,
        target_project=target_project,
        base_dir=base_dir,
        env=env,
        omnibus_s3_cache=False,
        log_level=log_level,
    )
