"""
Agentless-scanner tasks
"""


import os
import sys

from invoke import task
from invoke.exceptions import Exit

from tasks.agent import bundle_install_omnibus
from tasks.build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from tasks.flavor import AgentFlavor
from tasks.go import deps
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags, get_version, load_release_versions

# constants
AGENTLESS_SCANNER_BIN_PATH = os.path.join(".", "bin", "agentless-scanner")
STATIC_BIN_PATH = os.path.join(".", "bin", "static")


@task
def build(
    ctx,
    rebuild=False,
    race=False,
    static=False,
    build_include=None,
    build_exclude=None,
    major_version='7',
    arch="x64",
    go_mod="mod",
):
    """
    Build Agentless-scanner
    """
    build_include = (
        get_default_build_tags(build="agentless-scanner", arch=arch, flavor=AgentFlavor.agentless_scanner)
        if build_include is None
        else filter_incompatible_tags(build_include.split(","), arch=arch)
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    build_tags = get_build_tags(build_include, build_exclude)
    ldflags, gcflags, env = get_build_flags(ctx, static=static, major_version=major_version)
    bin_path = AGENTLESS_SCANNER_BIN_PATH

    if static:
        bin_path = STATIC_BIN_PATH

    # NOTE: consider stripping symbols to reduce binary size
    cmd = "go build -mod={go_mod} {race_opt} {build_type} -tags \"{build_tags}\" -o {bin_name} "
    cmd += "-gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/agentless-scanner"
    args = {
        "go_mod": go_mod,
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else "",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(bin_path, bin_name("agentless-scanner")),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args), env=env)

    # Render the configuration file template
    #
    # We need to remove cross compiling bits if any because go generate must
    # build and execute in the native platform
    env = {
        "GOOS": "",
        "GOARCH": "",
    }
    cmd = "go generate -mod={} {}/cmd/agentless-scanner"
    ctx.run(cmd.format(go_mod, REPO_PATH), env=env)

    if static and sys.platform.startswith("linux"):
        cmd = "file {bin_name} "
        args = {
            "bin_name": os.path.join(bin_path, bin_name("agentless-scanner")),
        }
        result = ctx.run(cmd.format(**args))
        if "statically linked" not in result.stdout:
            print("agentless-scanner binary is not static, exiting...")
            raise Exit(code=1)


@task
def omnibus_build(
    ctx,
    log_level="info",
    base_dir=None,
    gem_path=None,
    skip_deps=False,
    release_version="nightly",
    major_version='7',
    omnibus_s3_cache=False,
    go_mod_cache=None,
    host_distribution=None,
):
    """
    Build the Agentless-scanner packages with Omnibus Installer.
    """
    if not skip_deps:
        deps(ctx)

    # omnibus config overrides
    overrides = []

    # base dir (can be overridden through env vars, command line takes precedence)
    base_dir = base_dir or os.environ.get("ALS_OMNIBUS_BASE_DIR")
    if base_dir:
        overrides.append(f"base_dir:{base_dir}")
    if host_distribution:
        overrides.append(f'host_distribution:{host_distribution}')

    overrides_cmd = ""
    if overrides:
        overrides_cmd = "--override=" + " ".join(overrides)

    env = load_release_versions(ctx, release_version)

    env['PACKAGE_VERSION'] = get_version(
        ctx,
        include_git=True,
        url_safe=True,
        git_sha_length=7,
        major_version=major_version,
        include_pipeline_id=True,
    )

    bundle_install_omnibus(ctx, gem_path=gem_path, env=env)

    with ctx.cd("omnibus"):
        omnibus = "bundle exec omnibus.bat" if sys.platform == 'win32' else "bundle exec omnibus"
        cmd = "{omnibus} build agentless-scanner --log-level={log_level} {populate_s3_cache} {overrides}"
        args = {"omnibus": omnibus, "log_level": log_level, "overrides": overrides_cmd, "populate_s3_cache": ""}

        if omnibus_s3_cache:
            args['populate_s3_cache'] = " --populate-s3-cache "

        env['MAJOR_VERSION'] = major_version

        integrations_core_version = os.environ.get('INTEGRATIONS_CORE_VERSION')
        # Only overrides the env var if the value is a non-empty string.
        if integrations_core_version:
            env['INTEGRATIONS_CORE_VERSION'] = integrations_core_version

        # If the host has a GOMODCACHE set, try to reuse it
        if not go_mod_cache and os.environ.get('GOMODCACHE'):
            go_mod_cache = os.environ.get('GOMODCACHE')

        if go_mod_cache:
            env['OMNIBUS_GOMODCACHE'] = go_mod_cache

        ctx.run(cmd.format(**args), env=env)


@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/agentless-scanner folder
    print("Remove agentless-scanner binary folder")
    ctx.run("rm -rf ./bin/agentless-scanner")
