"""
installer namespaced tasks
"""

import os
import sys

from invoke import task

from tasks.agent import bundle_install_omnibus, render_config
from tasks.build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from tasks.flavor import AgentFlavor
from tasks.go import deps
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags, get_version, load_release_versions, timed

BIN_PATH = os.path.join(".", "bin", "installer")
MAJOR_VERSION = '7'


@task
def build(
    ctx,
    rebuild=False,
    race=False,
    install_path=None,
    build_include=None,
    build_exclude=None,
    arch="x64",
    go_mod="mod",
):
    """
    Build the updater.
    """

    ldflags, gcflags, env = get_build_flags(ctx, major_version=MAJOR_VERSION, install_path=install_path)

    build_include = (
        get_default_build_tags(
            build="updater",
        )  # TODO/FIXME: Arch not passed to preserve build tags. Should this be fixed?
        if build_include is None
        else filter_incompatible_tags(build_include.split(","), arch=arch)
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    build_tags = get_build_tags(build_include, build_exclude)

    race_opt = "-race" if race else ""
    build_type = "-a" if rebuild else ""
    go_build_tags = " ".join(build_tags)
    updater_bin = os.path.join(BIN_PATH, bin_name("installer"))
    cmd = f"go build -mod={go_mod} {race_opt} {build_type} -tags \"{go_build_tags}\" "
    cmd += f"-o {updater_bin} -gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/updater"

    ctx.run(cmd, env=env)

    helper_bin = os.path.join(BIN_PATH, bin_name("helper"))
    helper_ldflags = f"-X main.installPath={install_path} -w -s"
    helper_path = os.path.join("pkg", "updater", "service", "helper")
    cmd = f"CGO_ENABLED=0 go build {build_type} -tags \"{go_build_tags}\" "
    cmd += f"-o {helper_bin} -gcflags=\"{gcflags}\" -ldflags=\"{helper_ldflags}\" {helper_path}/main.go"

    ctx.run(cmd, env=env)
    render_config(
        ctx,
        env,
        flavor=AgentFlavor.base,
        python_runtimes='3',
        skip_assets=False,
        build_tags=build_tags,
        development=True,
        windows_sysprobe=False,
    )


def get_omnibus_env(
    ctx,
    skip_sign=False,
    release_version="nightly",
    hardened_runtime=False,
    go_mod_cache=None,
):
    env = load_release_versions(ctx, release_version)

    # If the host has a GOMODCACHE set, try to reuse it
    if not go_mod_cache and os.environ.get('GOMODCACHE'):
        go_mod_cache = os.environ.get('GOMODCACHE')

    if go_mod_cache:
        env['OMNIBUS_GOMODCACHE'] = go_mod_cache

    env['OMNIBUS_OPENSSL_SOFTWARE'] = 'openssl3'

    env_override = ['INTEGRATIONS_CORE_VERSION', 'OMNIBUS_SOFTWARE_VERSION']
    for key in env_override:
        value = os.environ.get(key)
        # Only overrides the env var if the value is a non-empty string.
        if value:
            env[key] = value

    if sys.platform == 'darwin':
        # Target MacOS 10.12
        env['MACOSX_DEPLOYMENT_TARGET'] = '10.12'

    if skip_sign:
        env['SKIP_SIGN_MAC'] = 'true'
    if hardened_runtime:
        env['HARDENED_RUNTIME_MAC'] = 'true'

    env['PACKAGE_VERSION'] = get_version(
        ctx, include_git=True, url_safe=True, major_version=MAJOR_VERSION, include_pipeline_id=True
    )
    env['MAJOR_VERSION'] = MAJOR_VERSION

    return env


def omnibus_run_task(ctx, task, target_project, base_dir, env, omnibus_s3_cache=False, log_level="info"):
    with ctx.cd("omnibus"):
        overrides_cmd = ""
        if base_dir:
            overrides_cmd = f"--override=base_dir:{base_dir}"

        omnibus = "bundle exec omnibus"
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


# hardened-runtime needs to be set to False to build on MacOS < 10.13.6, as the -o runtime option is not supported.
@task(
    help={
        'skip-sign': "On macOS, use this option to build an unsigned package if you don't have Datadog's developer keys.",
        'hardened-runtime': "On macOS, use this option to enforce the hardened runtime setting, adding '-o runtime' to all codesign commands",
    }
)
def omnibus_build(
    ctx,
    log_level="info",
    base_dir=None,
    gem_path=None,
    skip_deps=False,
    skip_sign=False,
    release_version="nightly",
    omnibus_s3_cache=False,
    hardened_runtime=False,
    go_mod_cache=None,
):
    """
    Build the Agent packages with Omnibus Installer.
    """
    if not skip_deps:
        with timed(quiet=True) as deps_elapsed:
            deps(ctx)

    # base dir (can be overridden through env vars, command line takes precedence)
    base_dir = base_dir or os.environ.get("OMNIBUS_BASE_DIR")

    env = get_omnibus_env(
        ctx,
        skip_sign=skip_sign,
        release_version=release_version,
        hardened_runtime=hardened_runtime,
        go_mod_cache=go_mod_cache,
    )

    target_project = "installer"

    with timed(quiet=True) as bundle_elapsed:
        bundle_install_omnibus(ctx, gem_path, env)

    with timed(quiet=True) as omnibus_elapsed:
        omnibus_run_task(
            ctx=ctx,
            task="build",
            target_project=target_project,
            base_dir=base_dir,
            env=env,
            omnibus_s3_cache=omnibus_s3_cache,
            log_level=log_level,
        )

    print("Build component timing:")
    if not skip_deps:
        print(f"Deps:    {deps_elapsed.duration}")
    print(f"Bundle:  {bundle_elapsed.duration}")
    print(f"Omnibus: {omnibus_elapsed.duration}")
