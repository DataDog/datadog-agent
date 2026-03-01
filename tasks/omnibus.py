import hashlib
import os
import re
import subprocess
import sys
import tempfile
import warnings
from collections import defaultdict
from collections.abc import Iterator
from typing import NamedTuple

import requests
from invoke import task
from invoke.exceptions import Exit, UnexpectedExit

from tasks.flavor import AgentFlavor
from tasks.go import deps
from tasks.libs.common.check_tools_version import expected_go_repo_v
from tasks.libs.common.omnibus import (
    ENV_PASSHTROUGH,
    OS_SPECIFIC_ENV_PASSTHROUGH,
    install_dir_for_project,
    omnibus_compute_cache_key,
    send_build_metrics,
    send_cache_miss_event,
    send_cache_mutation_event,
    should_retry_bundle_install,
)
from tasks.libs.common.user_interactions import yes_no_question
from tasks.libs.common.utils import gitlab_section, timed
from tasks.libs.dependencies import get_effective_dependencies_env
from tasks.libs.releasing.version import get_version


def omnibus_run_task(ctx, task, target_project, base_dir, env, log_level="info", host_distribution=None):
    with ctx.cd("omnibus"):
        overrides = []
        if base_dir:
            overrides.append(f"--override=base_dir:{base_dir}")
        if host_distribution:
            overrides.append(f"--override=host_distribution:{host_distribution}")

        omnibus = f"bundle exec {'omnibus.bat' if sys.platform == 'win32' else 'omnibus'}"
        cmd = "{omnibus} {task} {project_name} --log-level={log_level} {overrides}"
        args = {
            "omnibus": omnibus,
            "task": task,
            "project_name": target_project,
            "log_level": log_level,
            "overrides": " ".join(overrides),
        }

        with gitlab_section(f"Running omnibus task {task}", collapsed=True):
            ctx.run(cmd.format(**args), env=env, replace_env=True, err_stream=sys.stdout)


def bundle_install_omnibus(ctx, gem_path=None, env=None, max_try=2):
    with ctx.cd("omnibus"):
        # make sure bundle install starts from a clean state
        try:
            os.remove("Gemfile.lock")
        except FileNotFoundError:
            # It's okay if the file doesn't exist - we just want to ensure it's not there
            pass

        cmd = "bundle install"
        if gem_path:
            cmd += f" --path {gem_path}"

        with gitlab_section("Bundle install omnibus", collapsed=True):
            for trial in range(max_try):
                try:
                    ctx.run(cmd, env=env, replace_env=True, err_stream=sys.stdout)
                    return
                except UnexpectedExit as e:
                    if not should_retry_bundle_install(e.result):
                        print(f'Fatal error while installing omnibus: {e.result.stderr}. Cannot continue.')
                        raise
                    print(f"Retrying bundle install, attempt {trial + 1}/{max_try}")
        raise Exit('Too many failures while installing omnibus, giving up')


def get_omnibus_env(
    ctx,
    skip_sign=False,
    hardened_runtime=False,
    system_probe_bin=None,
    with_sd_agent=False,
    with_dd_procmgrd=False,
    go_mod_cache=None,
    flavor=AgentFlavor.base,
    pip_config_file="pip.conf",
    custom_config_dir=None,
    fips_mode=False,
):
    env = get_effective_dependencies_env()

    # If the host has a GOMODCACHE set, try to reuse it
    if not go_mod_cache and os.environ.get('GOMODCACHE'):
        go_mod_cache = os.environ.get('GOMODCACHE')

    if go_mod_cache:
        env['OMNIBUS_GOMODCACHE'] = go_mod_cache

    external_repos = {
        "INTEGRATIONS_CORE_VERSION": "https://github.com/DataDog/integrations-core.git",
        "OMNIBUS_RUBY_VERSION": "https://github.com/DataDog/omnibus-ruby.git",
    }
    for key, url in external_repos.items():
        ref = env[key]
        if not re.fullmatch(r"[0-9a-f]{4,40}", ref):  # resolve only "moving" refs, such as `own/branch`
            candidates = [
                line.split()
                for line in subprocess.check_output(["git", "ls-remote", "--refs", url, ref], text=True).splitlines()
            ]
            if not candidates:
                raise Exit(f"{key!r}: no candidate for {ref!r} @ {url}!")
            if len(candidates) > 1:  # happens when a branch name mimics its base or target, such as `my/own/branch`
                warnings.warn(
                    f"{key!r}: multiple candidates for {ref!r} @ {url} {[c[1] for c in candidates]}", stacklevel=1
                )
            sha1, shortest_ref = min(candidates, key=lambda c: len(c[1]))
            print(f"{key!r}: {ref!r} @ {url} resolves to {shortest_ref!r} -> {sha1}")
            env[key] = sha1

    if sys.platform == 'darwin':
        env['MACOSX_DEPLOYMENT_TARGET'] = '11.0'  # https://docs.datadoghq.com/agent/supported_platforms/?tab=macos

        if skip_sign:
            env['SKIP_SIGN_MAC'] = 'true'
        else:
            env['SIGN_MAC'] = 'true'
        if hardened_runtime:
            env['HARDENED_RUNTIME_MAC'] = 'true'

    env['PACKAGE_VERSION'] = get_version(ctx, include_git=True, url_safe=True, include_pipeline_id=True)

    # Since omnibus and the invoke task won't run in the same folder
    # we need to input the absolute path of the pip config file
    env['PIP_CONFIG_FILE'] = os.path.abspath(pip_config_file)

    if system_probe_bin:
        env['SYSTEM_PROBE_BIN'] = system_probe_bin
    if with_sd_agent:
        env['WITH_SD_AGENT'] = 'true'
    if with_dd_procmgrd:
        env['WITH_DD_PROCMGRD'] = 'true'
    env['AGENT_FLAVOR'] = flavor.name

    if custom_config_dir:
        env["OUTPUT_CONFIG_DIR"] = custom_config_dir

    if fips_mode:
        env['FIPS_MODE'] = 'true'
        if sys.platform == 'win32' and not os.environ.get('MSGO_ROOT'):
            # Point omnibus at the msgo root
            # TODO: idk how to do this in omnibus datadog-agent.rb
            #       because `File.read` is executed when the script is loaded,
            #       not when the `command`s are run and the source tree is not
            #       available at that time.
            #       Comments from the Linux FIPS PR discussed wanting to centralize
            #       the msgo root logic, so this can be updated then.
            go_version = expected_go_repo_v()
            env['MSGO_ROOT'] = f'C:\\msgo\\{go_version}\\go'
            gobinpath = f"{env['MSGO_ROOT']}\\bin\\go.exe"
            if not os.path.exists(gobinpath):
                raise Exit(f"msgo go.exe not found at {gobinpath}")

    # We need to override the workers variable in omnibus build when running on Kubernetes runners,
    # otherwise, ohai detect the number of CPU on the host and run the make jobs with all the CPU.
    kubernetes_cpu_request = os.environ.get('KUBERNETES_CPU_REQUEST')
    if kubernetes_cpu_request:
        env['OMNIBUS_WORKERS_OVERRIDE'] = str(int(kubernetes_cpu_request) + 1)

    env_to_forward = _passthrough_env_for_os(os.environ, sys.platform)
    env.update(env_to_forward)

    return env


def _passthrough_env_for_os(starting_env: dict[str, str], platform: str) -> dict[str, str]:
    expected_env = set(ENV_PASSHTROUGH) | set(OS_SPECIFIC_ENV_PASSTHROUGH[platform])

    missing_env = expected_env - set(starting_env)
    if missing_env:
        warnings.warn(
            f'Missing expected environment variables for Omnibus build: {missing_env}',
            stacklevel=1,
        )
    passthrough_env = {k: v for k, v in starting_env.items() if k in expected_env}

    return passthrough_env


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
    log_level="info",
    base_dir=None,
    gem_path=None,
    skip_deps=False,
    skip_sign=False,
    hardened_runtime=False,
    system_probe_bin=None,
    with_sd_agent=False,
    with_dd_procmgrd=False,
    go_mod_cache=None,
    python_mirror=None,
    pip_config_file="pip.conf",
    host_distribution=None,
    install_directory=None,
    config_directory=None,
    target_project=None,
):
    """
    Build the Agent packages with Omnibus Installer.
    """

    flavor = AgentFlavor[flavor]
    fips_mode = flavor.is_fips()
    durations = {}
    if not skip_deps:
        with timed(quiet=True) as durations['Deps']:
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
        hardened_runtime=hardened_runtime,
        system_probe_bin=system_probe_bin,
        with_sd_agent=with_sd_agent,
        with_dd_procmgrd=with_dd_procmgrd,
        go_mod_cache=go_mod_cache,
        flavor=flavor,
        pip_config_file=pip_config_file,
        custom_config_dir=config_directory,
        fips_mode=fips_mode,
    )

    if not target_project:
        target_project = "agent"

    if flavor != AgentFlavor.base and target_project not in ["agent", "ddot"]:
        print("flavors only make sense when building the agent or ddot")
        raise Exit(code=1)
    if flavor.is_iot():
        target_project = "iot-agent"

    # Get the python_mirror from the PIP_INDEX_URL environment variable if it is not passed in the args
    python_mirror = python_mirror or os.environ.get("PIP_INDEX_URL")

    # If a python_mirror is set then use it for pip by adding it in the pip.conf file
    pip_index_url = f"[global]\nindex-url = {python_mirror}" if python_mirror else ""

    # We're passing the --index-url arg through a pip.conf file so that omnibus doesn't leak the token
    with open(pip_config_file, 'w') as f:
        f.write(pip_index_url)

    with timed(quiet=True) as durations['Bundle']:
        bundle_install_omnibus(ctx, gem_path, env)

    omnibus_cache_dir = os.environ.get('OMNIBUS_GIT_CACHE_DIR')
    use_omnibus_git_cache = (
        omnibus_cache_dir is not None
        and target_project == "agent"
        and host_distribution != "ociru"
        and "OMNIBUS_PACKAGE_ARTIFACT_DIR" not in os.environ
    )
    remote_cache_name = os.environ.get('CI_JOB_NAME_SLUG')
    use_remote_cache = use_omnibus_git_cache and remote_cache_name is not None
    cache_state = None
    aws_cmd = "aws.exe" if sys.platform == 'win32' else "aws"
    if use_omnibus_git_cache:
        # The cache will be written in the provided cache dir (see omnibus.rb) but
        # the git repository itself will be located in a subfolder that replicates
        # the install_dir hierarchy
        # For instance if git_cache_dir is set to "/git/cache/dir" and install_dir is
        # set to /a/b/c, the cache git repository will be located in
        # /git/cache/dir/a/b/c/.git
        if not install_directory:
            install_directory = install_dir_for_project(target_project)
        # Is the path starts with a /, it's considered the new root for the joined path
        # which effectively drops whatever was in omnibus_cache_dir
        install_directory = install_directory.lstrip('/')
        omnibus_cache_dir = os.path.join(omnibus_cache_dir, install_directory)
        # We don't want to update the cache when not running on a CI
        # Individual developers are still able to leverage the cache by providing
        # the OMNIBUS_GIT_CACHE_DIR env variable, but they won't pull from the CI
        # generated one.
        with gitlab_section("Manage omnibus cache", collapsed=True):
            if use_remote_cache:
                cache_key = omnibus_compute_cache_key(ctx, env)
                git_cache_url = f"s3://{os.environ['S3_OMNIBUS_GIT_CACHE_BUCKET']}/{cache_key}/{remote_cache_name}"
                bundle_dir = tempfile.TemporaryDirectory()
                bundle_path = os.path.join(bundle_dir.name, 'omnibus-git-cache-bundle')
                with timed(quiet=True) as durations['Restoring omnibus cache']:
                    # Allow failure in case the cache was evicted
                    if ctx.run(f"{aws_cmd} s3 cp --only-show-errors {git_cache_url} {bundle_path}", warn=True):
                        print(f'Successfully retrieved cache {cache_key}')
                        try:
                            ctx.run(f"git clone --mirror {bundle_path} {omnibus_cache_dir}")
                        except UnexpectedExit as exc:
                            print(f"An error occurring while cloning the cache repo: {exc}")
                        else:
                            cache_state = ctx.run(f"git -C {omnibus_cache_dir} tag -l").stdout
                    else:
                        print(f'Failed to restore cache from key {cache_key}')
                        send_cache_miss_event(
                            ctx, os.environ.get('CI_PIPELINE_ID'), remote_cache_name, os.environ.get('CI_JOB_ID')
                        )

    with timed(quiet=True) as durations['Omnibus']:
        omni_flavor = env.get('AGENT_FLAVOR')
        print(f'We are building omnibus with flavor: {omni_flavor}')
        omnibus_run_task(
            ctx=ctx,
            task="build",
            target_project=target_project,
            base_dir=base_dir,
            env=env,
            log_level=log_level,
            host_distribution=host_distribution,
        )

    # Delete the temporary pip.conf file once the build is done
    os.remove(pip_config_file)

    if use_omnibus_git_cache:
        stale_tags = ctx.run(f'git -C {omnibus_cache_dir} tag --no-merged', warn=True).stdout
        # Purge the cache manually as omnibus will stick to not restoring a tag when
        # a mismatch is detected, but will keep the old cached tags.
        # Do this before checking for tag differences, in order to remove stale tags
        # in case they were included in the bundle in a previous build
        for _, tag in enumerate(stale_tags.split(os.linesep)):
            ctx.run(f'git -C {omnibus_cache_dir} tag -d {tag}')
        if use_remote_cache:
            if cache_state is None:
                with timed(quiet=True) as durations['Updating omnibus cache']:
                    ctx.run(f"git -C {omnibus_cache_dir} bundle create {bundle_path} --tags")
                    ctx.run(f"{aws_cmd} s3 cp --only-show-errors {bundle_path} {git_cache_url}")
                    bundle_dir.cleanup()
            elif ctx.run(f"git -C {omnibus_cache_dir} tag -l").stdout != cache_state:
                try:
                    send_cache_mutation_event(
                        ctx, os.environ.get('CI_PIPELINE_ID'), remote_cache_name, os.environ.get('CI_JOB_ID')
                    )
                except Exception as e:
                    print("Failed to send cache mutation event:", e)

    # Output duration information for different steps
    print("Build component timing:")
    durations_to_print = ["Deps", "Bundle", "Omnibus", "Restoring omnibus cache", "Updating omnibus cache"]
    for name in durations_to_print:
        if name in durations:
            print(f"{name}: {durations[name].duration}")

    try:
        send_build_metrics(ctx, durations['Omnibus'].duration)
    except Exception as e:
        print("Failed to send metrics:", e)


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
    hardened_runtime=False,
    system_probe_bin=None,
    with_sd_agent=False,
    with_dd_procmgrd=False,
    go_mod_cache=None,
):
    flavor = AgentFlavor[flavor]
    # base dir (can be overridden through env vars, command line takes precedence)
    base_dir = base_dir or os.environ.get("OMNIBUS_BASE_DIR")

    env = get_omnibus_env(
        ctx,
        skip_sign=skip_sign,
        hardened_runtime=hardened_runtime,
        system_probe_bin=system_probe_bin,
        with_sd_agent=with_sd_agent,
        with_dd_procmgrd=with_dd_procmgrd,
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
        log_level=log_level,
    )


@task()
def build_repackaged_agent(ctx, log_level="info"):
    """
    Create an Agent package by using an existing Agent package as a base and rebuilding the Agent binaries with the local checkout.

    Currently only expected to work for debian packages, and requires the `dpkg` command to be available.
    """
    # Make sure we let the user know that we're going to overwrite the existing Agent installation if present
    agent_path = "/opt/datadog-agent"
    if os.path.exists(agent_path):
        if not yes_no_question(
            f"The Agent installation directory {agent_path} already exists, and will be overwritten by this build. Continue?",
            color="red",
            default=False,
        ):
            raise Exit("Operation cancelled")

        import shutil

        shutil.rmtree("/opt/datadog-agent")

    architecture = ctx.run("dpkg --print-architecture", hide=True).stdout.strip()

    # Fetch the Packages file from the nightly repository and get the datadog-agent package with the highest pipeline ID
    # The assumption here is that only nightlies from master are pushed to the nightly repository
    # and that simply picking up the highest pipeline ID will give us what we want without having to query Gitlab.
    packages_url = f"https://apt.datad0g.com/dists/nightly/7/binary-{architecture}/Packages"
    with requests.get(packages_url, stream=True, timeout=10) as response:
        response.raise_for_status()
        lines = response.iter_lines(decode_unicode=True)

        latest_package = max(
            (pkg for pkg in _packages_from_deb_metadata(lines) if pkg.package_name == "datadog-agent"),
            key=_pipeline_id_of_package,
        )

    env = get_omnibus_env(ctx, skip_sign=True, flavor=AgentFlavor.base)

    env['OMNIBUS_REPACKAGE_SOURCE_URL'] = f"https://apt.datad0g.com/{latest_package.filename}"
    env['OMNIBUS_REPACKAGE_SOURCE_SHA256'] = latest_package.sha256
    # Set up compiler flags (assumes an environment based on our glibc-targeting toolchains)
    if architecture == "amd64":
        env.update(
            {
                "DD_CC": "x86_64-unknown-linux-gnu-gcc",
                "DD_CXX": "x86_64-unknown-linux-gnu-g++",
                "DD_CMAKE_TOOLCHAIN": "/opt/cmake/x86_64-unknown-linux-gnu.toolchain.cmake",
            }
        )
    elif architecture == "arm64":
        env.update(
            {
                "DD_CC": "aarch64-unknown-linux-gnu-gcc",
                "DD_CXX": "aarch64-unknown-linux-gnu-g++",
                "DD_CMAKE_TOOLCHAIN": "/opt/cmake/aarch64-unknown-linux-gnu.toolchain.cmake",
            }
        )

    print("Using the following package as a base:", env['OMNIBUS_REPACKAGE_SOURCE_URL'])

    bundle_install_omnibus(ctx, None, env)

    omnibus_run_task(ctx, "build", "agent", base_dir=None, env=env, log_level=log_level)


class DebPackageInfo(NamedTuple):
    package_name: str | None
    filename: str | None
    sha256: str | None

    @classmethod
    def from_metadata(cls, package_info: dict) -> "DebPackageInfo":
        """Creates a DebPackageInfo object from a dictionary of package metadata."""
        return cls(
            package_name=package_info.get("Package"),
            filename=package_info.get("Filename"),
            sha256=package_info.get("SHA256"),
        )


def _packages_from_deb_metadata(lines: Iterator[str]) -> Iterator[DebPackageInfo]:
    """Generator function that yields package blocks from the lines of a deb Packages metadata file."""
    package_info = {}
    for line in lines:
        # Empty line indicates end of package block
        if not line.strip():
            if package_info:
                yield DebPackageInfo.from_metadata(package_info)
                package_info = {}  # Reset for next package
            continue

        try:
            key, value = line.split(":", 1)
            package_info[key] = value.strip()
        except ValueError:
            continue

    # Don't forget the last package if it exists
    if package_info:
        yield DebPackageInfo.from_metadata(package_info)


@task(
    help={
        'arch': "Target architecture: 'arm64' or 'amd64' (default: auto-detect from host)",
        'cache-dir': "Base directory for caches (default: ~/.omnibus-docker-cache)",
        'workers': "Number of parallel workers for compression and builds (default: 8)",
        'build-image': "Docker build image to use (default: uses version from .gitlab-ci.yml)",
        'tag': "Tag for the built Docker image (default: localhost/datadog-agent:local)",
    }
)
def docker_build(
    ctx,
    arch=None,
    cache_dir=None,
    workers=8,
    build_image=None,
    tag="localhost/datadog-agent:local",
):
    """
    Build the Agent inside a Docker container and create a runnable Docker image.

    This is ideal for local development when you want production-like builds without
    setting up a local omnibus/Ruby environment. It handles Docker Desktop VirtioFS
    quirks and uses the same buildimages as CI.

    Related tasks:
    - omnibus.build: For CI pipelines or when you have local omnibus/Ruby setup
    - agent.image-build: Creates Docker image from existing omnibus deb package
    - agent.hacky-dev-image-build: Quick iteration with locally-built binaries (not omnibus)

    This task:
    1. Runs the omnibus build inside a Docker container (with caching)
    2. Creates a Docker image from the build output
    3. Loads the image into your local Docker daemon

    Examples:
        # Build and create Docker image (default)
        dda inv omnibus.docker-build

        # Build with custom image tag
        dda inv omnibus.docker-build --tag=my-agent:dev

        # Build for specific architecture
        dda inv omnibus.docker-build --arch=arm64
    """
    import glob
    import platform as plat

    # Auto-detect architecture if not specified
    if arch is None:
        machine = plat.machine().lower()
        if machine in ('arm64', 'aarch64'):
            arch = 'arm64'
        elif machine in ('x86_64', 'amd64'):
            arch = 'amd64'
        else:
            raise Exit(f"Unknown architecture: {machine}. Please specify --arch=arm64 or --arch=amd64")

    # Map architecture to cross-compiler triplet
    if arch == 'arm64':
        cc = 'aarch64-unknown-linux-gnu-gcc'
        cxx = 'aarch64-unknown-linux-gnu-g++'
    elif arch == 'amd64':
        cc = 'x86_64-unknown-linux-gnu-gcc'
        cxx = 'x86_64-unknown-linux-gnu-g++'
    else:
        raise Exit(f"Invalid architecture: {arch}. Use 'arm64' or 'amd64'")

    # Resolve build image using version from .gitlab-ci.yml if not specified
    if build_image is None:
        from tasks.buildimages import get_tag

        image_tag = get_tag(ctx, image_type="linux")
        build_image = f"registry.ddbuild.io/ci/datadog-agent-buildimages/linux:{image_tag}"

    # Set up cache directories with intelligent defaults for Workspaces
    if cache_dir is None:
        # Detect Workspaces environment (has high-performance /instance_storage SSD mount)
        if os.path.exists("/instance_storage") and os.path.isdir("/instance_storage"):
            cache_dir = "/instance_storage/omnibus-docker-cache"
            print("Detected Workspaces environment, using high-performance storage: /instance_storage")
        else:
            # Default to home directory for local development
            home = os.path.expanduser("~")
            cache_dir = os.path.join(home, ".omnibus-docker-cache")

    omnibus_dir = os.path.join(cache_dir, "omnibus")
    gems_dir = os.path.join(cache_dir, "gems")
    go_mod_dir = os.path.join(cache_dir, "go-mod")
    go_build_dir = os.path.join(cache_dir, "go-build")

    # VIRTIO-FS WORKAROUND: Single Volume for Git Cache + Install Dir
    #
    # Docker Desktop's VirtioFS can corrupt git objects when operations
    # span multiple bind mounts. Omnibus's git cache uses --git-dir separate
    # from --work-tree, which normally means reads from /opt/datadog-agent
    # and writes to /omnibus-git-cache cross volume boundaries.
    #
    # VirtioFS may process I/O to different mounts through independent
    # channels, causing ordering issues that corrupt git's loose objects.
    #
    # Solution: Put both directories under ONE bind mount, use a symlink
    # to maintain the expected /opt/datadog-agent path:
    #
    #   Host directory:
    #     ~/.omnibus-docker-cache/omnibus-state/git-cache/opt/datadog-agent/  (git objects)
    #     ~/.omnibus-docker-cache/omnibus-state/opt/datadog-agent/            (build artifacts)
    #
    #   Container mounts and symlinks:
    #     MOUNT: ~/.omnibus-docker-cache/omnibus-state/ -> /omnibus-state/
    #     SYMLINK: /opt/datadog-agent -> /omnibus-state/opt/datadog-agent/
    #
    # See: https://github.com/docker/for-mac/issues/7494
    #      https://docs.kernel.org/filesystems/virtiofs.html
    #
    omnibus_state_dir = os.path.join(cache_dir, "omnibus-state")
    git_cache_subdir = os.path.join(omnibus_state_dir, "git-cache")
    opt_subdir = os.path.join(omnibus_state_dir, "opt", "datadog-agent")

    # Create directories if they don't exist
    for d in [omnibus_dir, omnibus_state_dir, git_cache_subdir, opt_subdir, gems_dir, go_mod_dir, go_build_dir]:
        os.makedirs(d, exist_ok=True)

    # Get current working directory (repo root)
    repo_root = os.getcwd()

    # Build environment variables
    env_args = [
        "-e OMNIBUS_GIT_CACHE_DIR=/omnibus-state/git-cache",
        f"-e OMNIBUS_WORKERS_OVERRIDE={workers}",
        f"-e DD_CC={cc}",
        f"-e DD_CXX={cxx}",
        # Set git safe.directory via env vars (doesn't persist to global config)
        "-e GIT_CONFIG_COUNT=1",
        "-e GIT_CONFIG_KEY_0=safe.directory",
        "-e GIT_CONFIG_VALUE_0=/go/src/github.com/DataDog/datadog-agent",
        # Skip XZ compression - faster for local dev, use omnibus.build for CI
        "-e SKIP_PKG_COMPRESSION=true",
    ]

    # Build volume mounts (note: /opt/datadog-agent is a symlink created in build_cmd)
    volume_args = [
        f"-v {omnibus_dir}:/omnibus",
        f"-v {omnibus_state_dir}:/omnibus-state",
        f"-v {gems_dir}:/gems",
        f"-v {go_mod_dir}:/go/pkg/mod",
        f"-v {go_build_dir}:/root/.cache/go-build",
        f"-v {repo_root}:/go/src/github.com/DataDog/datadog-agent",
    ]

    # Build the docker command
    env_str = " ".join(env_args)
    vol_str = " ".join(volume_args)

    # Create symlink for /opt/datadog-agent pointing to the single volume mount
    # This ensures git-cache and install-dir share the same VirtioFS I/O channel
    build_cmd = (
        'bash -c "'
        'mkdir -p /omnibus-state/opt/datadog-agent && '
        'rm -rf /opt/datadog-agent && '
        'ln -sfn /omnibus-state/opt/datadog-agent /opt/datadog-agent && '
        'dda inv -- -e omnibus.build --base-dir=/omnibus --gem-path=/gems'
        '"'
    )
    docker_cmd = (
        f"docker run --rm "
        f"{env_str} {vol_str} "
        f"-w /go/src/github.com/DataDog/datadog-agent "
        f"{build_image} "
        f"{build_cmd}"
    )

    print(f"Building Datadog Agent for {arch}")
    print(f"Build image: {build_image}")
    print(f"Cache directory: {cache_dir}")
    print(f"Workers: {workers}")
    print()

    # Run the omnibus build
    ctx.run(docker_cmd)

    # Build Docker image from the tarball
    print("\n" + "=" * 60)
    print("Building Docker image from tarball...")
    print("=" * 60 + "\n")

    artifacts_dir = os.path.join(omnibus_dir, "pkg")

    # Find the uncompressed tarball (we always set SKIP_PKG_COMPRESSION=true)
    tar_pattern = os.path.join(artifacts_dir, f"datadog-agent-*-{arch}.tar")
    tar_files = sorted(glob.glob(tar_pattern), key=os.path.getmtime, reverse=True)

    # Exclude debug packages
    tar_files = [f for f in tar_files if '-dbg-' not in f]

    if not tar_files:
        raise Exit(f"No tarball found matching {tar_pattern}. Build may have failed.")

    tarball = tar_files[0]

    artifact_name = os.path.basename(tarball)
    print(f"Using tarball: {tarball}")

    # Get git info for labels
    git_url = "https://github.com/DataDog/datadog-agent"
    git_sha = ctx.run("git rev-parse HEAD", hide=True).stdout.strip()

    # Map arch to Docker platform
    platform = f"linux/{arch}"

    # Build the Docker command
    build_args = [
        f"--build-arg DD_GIT_REPOSITORY_URL={git_url}",
        f"--build-arg DD_GIT_COMMIT_SHA={git_sha}",
        f"--build-arg DD_AGENT_ARTIFACT={artifact_name}",
    ]

    docker_build_cmd = (
        f"docker buildx build --platform {platform} "
        f"--build-context artifacts={artifacts_dir} "
        f"{' '.join(build_args)} "
        f"--file Dockerfiles/agent/Dockerfile "
        f"--tag {tag} "
        f"--load "
        f"Dockerfiles/agent"
    )

    ctx.run(docker_build_cmd)

    # Print clear usage instructions
    print("\n" + "=" * 60)
    print("BUILD COMPLETE")
    print("=" * 60)
    print(f"\nDocker image: {tag}")
    print("\nRun the agent:")
    print(f"  docker run --rm {tag} agent version")
    print(f"  docker run --rm -it {tag} /bin/bash")
    print("\nInspect the image:")
    print(f"  docker run --rm {tag} ls -la /opt/datadog-agent/bin/agent/")


def _pipeline_id_of_package(package: DebPackageInfo) -> int:
    """
    Returns the pipeline ID of the package, or -1 if the package doesn't have a pipeline ID.

    The filenames are expected to be in the format of pool/d/da/datadog-agent_<version>.pipeline.<pipeline_id>-1_<arch>.deb
    """
    pipeline_id_match = re.search(r'pipeline\.(\d+)', package.filename)
    if pipeline_id_match:
        return int(pipeline_id_match[1])
    return -1


def _otool_install_path_replacements(otool_output, install_path):
    """Returns a mapping of path replacements from `otool -l` output
    where references to `install_path` are replaced by `@rpath`."""
    for otool_line in otool_output.splitlines():
        if "name" not in otool_line:
            continue
        dylib_path = otool_line.strip().split(" ")[1]
        if install_path not in dylib_path:
            continue
        new_dylib_path = dylib_path.replace(f"{install_path}/embedded/lib", "@rpath")
        yield dylib_path, new_dylib_path


def _replace_dylib_paths_with_rpath(ctx, otool_output, install_path, file):
    for dylib_path, new_dylib_path in _otool_install_path_replacements(otool_output, install_path):
        ctx.run(f"install_name_tool -change {dylib_path} {new_dylib_path} {file}")


def _replace_dylib_id_paths_with_rpath(ctx, otool_output, install_path, file):
    for _, new_dylib_path in _otool_install_path_replacements(otool_output, install_path):
        ctx.run(f"install_name_tool -id {new_dylib_path} {file}")


def _patch_binary_rpath(ctx, new_rpath, install_path, binary_rpath, platform, file):
    if platform == "linux":
        ctx.run(f"patchelf --force-rpath --set-rpath \\$ORIGIN/{new_rpath}/embedded/lib {file}")
    else:
        # The macOS agent binary has 18 RPATH definition, replacing the first one should be enough
        # but just in case we're replacing them all.
        # We're also avoiding unnecessary `install_name_tool` call as much as possible.
        number_of_rpaths = binary_rpath.count('\n') // 3
        for _ in range(number_of_rpaths):
            exit_code = ctx.run(
                f"install_name_tool -rpath {install_path}/embedded/lib @loader_path/{new_rpath}/embedded/lib {file}",
                warn=True,
                hide=True,
            ).exited
            if exit_code != 0:
                break


@task
def rpath_edit(ctx, install_path, target_rpath_dd_folder, platform="linux"):
    # Collect mime types for all files inside the Agent installation
    files = ctx.run(rf"find {install_path} -type f -exec file --mime-type \{{\}} \+", hide=True).stdout
    for line in files.splitlines():
        if not line:
            continue
        file, file_type = line.split(":")
        file_type = file_type.strip()

        if platform == "linux":
            if file_type not in ["application/x-executable", "inode/symlink", "application/x-sharedlib"]:
                continue
            binary_rpath = ctx.run(f'objdump -x {file} | grep "RPATH"', warn=True, hide=True).stdout
        else:
            if file_type != "application/x-mach-binary":
                continue
            with tempfile.NamedTemporaryFile() as tmpfile:
                result = ctx.run(f'otool -l {file} > {tmpfile.name}', warn=True, hide=True)
                if result.exited:
                    continue
                binary_rpath = ctx.run(f'cat {tmpfile.name} | grep -A 2 "RPATH"', warn=True, hide=True).stdout
                dylib_paths = ctx.run(f'cat {tmpfile.name} | grep -A 2 "LC_LOAD_DYLIB"', warn=True, hide=True).stdout

                dylib_id_paths = ctx.run(f'cat {tmpfile.name} | grep -A 2 "LC_ID_DYLIB"', warn=True, hide=True).stdout

            # if a dylib ID use our installation path we replace it with @rpath instead
            if install_path in dylib_id_paths:
                _replace_dylib_id_paths_with_rpath(ctx, dylib_id_paths, install_path, file)

            # if a dylib use our installation path we replace it with @rpath instead
            if install_path in dylib_paths:
                _replace_dylib_paths_with_rpath(ctx, dylib_paths, install_path, file)

        # if a binary has an rpath that use our installation path we are patching it
        if install_path in binary_rpath:
            new_rpath = os.path.relpath(target_rpath_dd_folder, os.path.dirname(file))
            _patch_binary_rpath(ctx, new_rpath, install_path, binary_rpath, platform, file)


@task
def deduplicate_files(ctx, directory):
    # Matches: .so, .so.X, .so.X.Y, .so.X.Y.Z, .bundle, .dll, .dylib, .pyd
    LIB_PATTERN = re.compile(r"\.(bundle|dll|dylib|pyd|so(?:\.\d+)*)$")

    def hash_file(filepath):
        """Returns the SHA-256 hash of the file's contents."""
        with open(filepath, "rb") as f:
            return hashlib.file_digest(f, "sha256").hexdigest()

    def find_duplicates(root_dir):
        """Finds and returns duplicates as a map of hash -> list of files with that hash, excluding empty files."""
        hash_to_files = defaultdict(list)
        for dirpath, _, filenames in os.walk(root_dir):
            for name in filenames:
                if not LIB_PATTERN.search(name):
                    continue

                full_path = os.path.join(dirpath, name)
                # Only regular files; skip symlinks
                if os.path.isfile(full_path) and not os.path.islink(full_path):
                    try:
                        if os.path.getsize(full_path) == 0:
                            continue  # Exclude empty files
                        file_hash = hash_file(full_path)
                        hash_to_files[file_hash].append(full_path)
                    except Exception as e:
                        print(f"Error hashing {full_path}: {e}")
        return {h: paths for h, paths in hash_to_files.items() if len(paths) > 1}

    def replace_with_symlinks(duplicates):
        """Replaces all duplicates with symlinks to the first original (shortest path wins)."""
        for files in duplicates.values():
            files.sort(key=lambda p: (len(p), p))  # shortest path, then lexicographic
            original = files[0]
            for dup in files[1:]:
                try:
                    os.remove(dup)
                    rel_path = os.path.relpath(original, os.path.dirname(dup))
                    os.symlink(rel_path, dup)
                    print(f"Replaced {dup} with symlink to {original}")
                except Exception as e:
                    print(f"Failed to replace {dup}: {e}")

    root = os.path.abspath(directory)
    if not os.path.isdir(root):
        print(f"{root} is not a valid directory.")
        return

    print(f"Scanning for duplicates in: {root}")
    duplicates = find_duplicates(root)

    if not duplicates:
        return

    print(f"Found {sum(len(v) - 1 for v in duplicates.values())} duplicate files.")
    replace_with_symlinks(duplicates)
