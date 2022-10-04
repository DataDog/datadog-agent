"""
Agent namespaced tasks
"""


import ast
import datetime
import glob
import os
import platform
import re
import shutil
import sys
from distutils.dir_util import copy_tree

from invoke import task
from invoke.exceptions import Exit, ParseError

from .build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from .docker import pull_base_images
from .flavor import AgentFlavor
from .go import deps
from .rtloader import clean as rtloader_clean
from .rtloader import install as rtloader_install
from .rtloader import make as rtloader_make
from .ssm import get_pfx_pass, get_signing_cert
from .utils import (
    REPO_PATH,
    bin_name,
    generate_config,
    get_build_flags,
    get_version,
    get_version_numeric_only,
    get_win_py_runtime_var,
    has_both_python,
    load_release_versions,
)

# constants
BIN_PATH = os.path.join(".", "bin", "agent")
AGENT_TAG = "datadog/agent:master"

AGENT_CORECHECKS = [
    "container",
    "containerd",
    "cpu",
    "cri",
    "snmp",
    "docker",
    "file_handle",
    "go_expvar",
    "io",
    "jmx",
    "kubernetes_apiserver",
    "load",
    "memory",
    "ntp",
    "oom_kill",
    "systemd",
    "tcp_queue_length",
    "uptime",
    "winkmem",
    "winproc",
    "jetson",
]

IOT_AGENT_CORECHECKS = [
    "cpu",
    "disk",
    "io",
    "load",
    "memory",
    "network",
    "ntp",
    "uptime",
    "systemd",
    "jetson",
]

CACHED_WHEEL_FILENAME_PATTERN = "datadog_{integration}-*.whl"
CACHED_WHEEL_DIRECTORY_PATTERN = "integration-wheels/{branch}/{hash}/{python_version}/"
CACHED_WHEEL_FULL_PATH_PATTERN = CACHED_WHEEL_DIRECTORY_PATTERN + CACHED_WHEEL_FILENAME_PATTERN
LAST_DIRECTORY_COMMIT_PATTERN = "git -C {integrations_dir} rev-list -1 HEAD {integration}"


@task
def build(
    ctx,
    rebuild=False,
    race=False,
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
    development=True,
    skip_assets=False,
    embedded_path=None,
    rtloader_root=None,
    python_home_2=None,
    python_home_3=None,
    major_version='7',
    python_runtimes='3',
    arch='x64',
    exclude_rtloader=False,
    go_mod="mod",
    windows_sysprobe=False,
):
    """
    Build the agent. If the bits to include in the build are not specified,
    the values from `invoke.yaml` will be used.

    Example invokation:
        inv agent.build --build-exclude=systemd
    """
    flavor = AgentFlavor[flavor]

    if not exclude_rtloader and not flavor.is_iot():
        # If embedded_path is set, we should give it to rtloader as it should install the headers/libs
        # in the embedded path folder because that's what is used in get_build_flags()
        rtloader_make(ctx, python_runtimes=python_runtimes, install_prefix=embedded_path)
        rtloader_install(ctx)

    ldflags, gcflags, env = get_build_flags(
        ctx,
        embedded_path=embedded_path,
        rtloader_root=rtloader_root,
        python_home_2=python_home_2,
        python_home_3=python_home_3,
        major_version=major_version,
        python_runtimes=python_runtimes,
    )

    if sys.platform == 'win32':
        py_runtime_var = get_win_py_runtime_var(python_runtimes)

        windres_target = "pe-x86-64"

        # Important for x-compiling
        env["CGO_ENABLED"] = "1"

        if arch == "x86":
            env["GOARCH"] = "386"
            windres_target = "pe-i386"

        # This generates the manifest resource. The manifest resource is necessary for
        # being able to load the ancient C-runtime that comes along with Python 2.7
        # command = "rsrc -arch amd64 -manifest cmd/agent/agent.exe.manifest -o cmd/agent/rsrc.syso"
        ver = get_version_numeric_only(ctx, major_version=major_version)
        build_maj, build_min, build_patch = ver.split(".")

        command = f"windmc --target {windres_target} -r cmd/agent cmd/agent/agentmsg.mc "
        ctx.run(command, env=env)

        command = f"windres --target {windres_target} --define {py_runtime_var}=1 --define MAJ_VER={build_maj} --define MIN_VER={build_min} --define PATCH_VER={build_patch} --define BUILD_ARCH_{arch}=1"
        command += "-i cmd/agent/agent.rc -O coff -o cmd/agent/rsrc.syso"
        ctx.run(command, env=env)

    if flavor.is_iot():
        # Iot mode overrides whatever passed through `--build-exclude` and `--build-include`
        build_tags = get_default_build_tags(build="agent", arch=arch, flavor=flavor)
    else:
        build_include = (
            get_default_build_tags(build="agent", arch=arch, flavor=flavor)
            if build_include is None
            else filter_incompatible_tags(build_include.split(","), arch=arch)
        )
        build_exclude = [] if build_exclude is None else build_exclude.split(",")
        build_tags = get_build_tags(build_include, build_exclude)

    cmd = "go build -mod={go_mod} {race_opt} {build_type} -tags \"{go_build_tags}\" "

    cmd += "-o {agent_bin} -gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/{flavor}"
    args = {
        "go_mod": go_mod,
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else "",
        "go_build_tags": " ".join(build_tags),
        "agent_bin": os.path.join(BIN_PATH, bin_name("agent", android=False)),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
        "flavor": "iot-agent" if flavor.is_iot() else "agent",
    }
    ctx.run(cmd.format(**args), env=env)

    # Remove cross-compiling bits to render config
    env.update({"GOOS": "", "GOARCH": ""})

    # Render the Agent configuration file template
    build_type = "agent-py3"
    if flavor.is_iot():
        build_type = "iot-agent"
    elif has_both_python(python_runtimes):
        build_type = "agent-py2py3"

    generate_config(ctx, build_type=build_type, output_file="./cmd/agent/dist/datadog.yaml", env=env)

    # On Linux and MacOS, render the system-probe configuration file template
    if sys.platform != 'win32' or windows_sysprobe:
        generate_config(ctx, build_type="system-probe", output_file="./cmd/agent/dist/system-probe.yaml", env=env)

    if not skip_assets:
        refresh_assets(ctx, build_tags, development=development, flavor=flavor.name, windows_sysprobe=windows_sysprobe)


@task
def refresh_assets(_, build_tags, development=True, flavor=AgentFlavor.base.name, windows_sysprobe=False):
    """
    Clean up and refresh Collector's assets and config files
    """
    flavor = AgentFlavor[flavor]
    # ensure BIN_PATH exists
    if not os.path.exists(BIN_PATH):
        os.mkdir(BIN_PATH)

    dist_folder = os.path.join(BIN_PATH, "dist")
    if os.path.exists(dist_folder):
        shutil.rmtree(dist_folder)
    os.mkdir(dist_folder)

    if "python" in build_tags:
        copy_tree("./cmd/agent/dist/checks/", os.path.join(dist_folder, "checks"))
        copy_tree("./cmd/agent/dist/utils/", os.path.join(dist_folder, "utils"))
        shutil.copy("./cmd/agent/dist/config.py", os.path.join(dist_folder, "config.py"))
    if not flavor.is_iot():
        shutil.copy("./cmd/agent/dist/dd-agent", os.path.join(dist_folder, "dd-agent"))
        # copy the dd-agent placeholder to the bin folder
        bin_ddagent = os.path.join(BIN_PATH, "dd-agent")
        shutil.move(os.path.join(dist_folder, "dd-agent"), bin_ddagent)

    # System probe not supported on windows
    if sys.platform.startswith('linux') or windows_sysprobe:
        shutil.copy("./cmd/agent/dist/system-probe.yaml", os.path.join(dist_folder, "system-probe.yaml"))
    shutil.copy("./cmd/agent/dist/datadog.yaml", os.path.join(dist_folder, "datadog.yaml"))

    for check in AGENT_CORECHECKS if not flavor.is_iot() else IOT_AGENT_CORECHECKS:
        check_dir = os.path.join(dist_folder, f"conf.d/{check}.d/")
        copy_tree(f"./cmd/agent/dist/conf.d/{check}.d/", check_dir)
    if "apm" in build_tags:
        shutil.copy("./cmd/agent/dist/conf.d/apm.yaml.default", os.path.join(dist_folder, "conf.d/apm.yaml.default"))
    if "process" in build_tags:
        shutil.copy(
            "./cmd/agent/dist/conf.d/process_agent.yaml.default",
            os.path.join(dist_folder, "conf.d/process_agent.yaml.default"),
        )

    copy_tree("./cmd/agent/gui/views", os.path.join(dist_folder, "views"))
    if development:
        copy_tree("./dev/dist/", dist_folder)


@task
def run(
    ctx,
    rebuild=False,
    race=False,
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
    skip_build=False,
    config_path=None,
):
    """
    Execute the agent binary.

    By default it builds the agent before executing it, unless --skip-build was
    passed. It accepts the same set of options as agent.build.
    """
    if not skip_build:
        build(ctx, rebuild, race, build_include, build_exclude, flavor)

    agent_bin = os.path.join(BIN_PATH, bin_name("agent"))
    config_path = os.path.join(BIN_PATH, "dist", "datadog.yaml") if not config_path else config_path
    ctx.run(f"{agent_bin} run -c {config_path}")


@task
def system_tests(_):
    """
    Run the system testsuite.
    """
    pass


@task
def image_build(ctx, arch='amd64', base_dir="omnibus", python_version="2", skip_tests=False, signed_pull=True):
    """
    Build the docker image
    """
    BOTH_VERSIONS = ["both", "2+3"]
    VALID_VERSIONS = ["2", "3"] + BOTH_VERSIONS
    if python_version not in VALID_VERSIONS:
        raise ParseError("provided python_version is invalid")

    build_context = "Dockerfiles/agent"
    base_dir = base_dir or os.environ.get("OMNIBUS_BASE_DIR")
    pkg_dir = os.path.join(base_dir, 'pkg')
    deb_glob = f'datadog-agent*_{arch}.deb'
    dockerfile_path = f"{build_context}/{arch}/Dockerfile"
    list_of_files = glob.glob(os.path.join(pkg_dir, deb_glob))
    # get the last debian package built
    if not list_of_files:
        print(f"No debian package build found in {pkg_dir}")
        print("See agent.omnibus-build")
        raise Exit(code=1)
    latest_file = max(list_of_files, key=os.path.getctime)
    shutil.copy2(latest_file, build_context)
    # Pull base image with content trust enabled
    pull_base_images(ctx, dockerfile_path, signed_pull)
    common_build_opts = f"-t {AGENT_TAG} -f {dockerfile_path}"
    if python_version not in BOTH_VERSIONS:
        common_build_opts = f"{common_build_opts} --build-arg PYTHON_VERSION={python_version}"

    # Build with the testing target
    if not skip_tests:
        ctx.run(f"docker build {common_build_opts} --target testing {build_context}")

    # Build with the release target
    ctx.run(f"docker build {common_build_opts} --target release {build_context}")
    ctx.run(f"rm {build_context}/{deb_glob}")


@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False, go_mod="mod", arch="x64"):
    """
    Run integration tests for the Agent
    """
    if install_deps:
        deps(ctx)

    test_args = {
        "go_mod": go_mod,
        "go_build_tags": " ".join(get_default_build_tags(build="test", arch=arch)),
        "race_opt": "-race" if race else "",
        "exec_opts": "",
    }

    # since Go 1.13, the -exec flag of go test could add some parameters such as -test.timeout
    # to the call, we don't want them because while calling invoke below, invoke
    # thinks that the parameters are for it to interpret.
    # we're calling an intermediate script which only pass the binary name to the invoke task.
    if remote_docker:
        test_args["exec_opts"] = f"-exec \"{os.getcwd()}/test/integration/dockerize_tests.sh\""

    go_cmd = 'go test -mod={go_mod} {race_opt} -tags "{go_build_tags}" {exec_opts}'.format(**test_args)  # noqa: FS002

    prefixes = [
        "./test/integration/config_providers/...",
        "./test/integration/corechecks/...",
        "./test/integration/listeners/...",
        "./test/integration/util/kubelet/...",
    ]

    for prefix in prefixes:
        ctx.run(f"{go_cmd} {prefix}")


def get_omnibus_env(
    ctx,
    skip_sign=False,
    release_version="nightly",
    major_version='7',
    python_runtimes='3',
    hardened_runtime=False,
    system_probe_bin=None,
    nikos_path=None,
    go_mod_cache=None,
    flavor=AgentFlavor.base,
):
    env = load_release_versions(ctx, release_version)

    # If the host has a GOMODCACHE set, try to reuse it
    if not go_mod_cache and os.environ.get('GOMODCACHE'):
        go_mod_cache = os.environ.get('GOMODCACHE')

    if go_mod_cache:
        env['OMNIBUS_GOMODCACHE'] = go_mod_cache

    integrations_core_version = os.environ.get('INTEGRATIONS_CORE_VERSION')
    # Only overrides the env var if the value is a non-empty string.
    if integrations_core_version:
        env['INTEGRATIONS_CORE_VERSION'] = integrations_core_version

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
    if system_probe_bin:
        env['SYSTEM_PROBE_BIN'] = system_probe_bin
    if nikos_path:
        env['NIKOS_PATH'] = nikos_path
    env['AGENT_FLAVOR'] = flavor.name

    return env


def omnibus_run_task(ctx, task, target_project, base_dir, env, omnibus_s3_cache=False, log_level="info"):
    with ctx.cd("omnibus"):
        overrides_cmd = ""
        if base_dir:
            overrides_cmd = f"--override=base_dir:{base_dir}"

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


def bundle_install_omnibus(ctx, gem_path=None, env=None):
    with ctx.cd("omnibus"):
        # make sure bundle install starts from a clean state
        try:
            os.remove("Gemfile.lock")
        except Exception:
            pass

        cmd = "bundle install"
        if gem_path:
            cmd += f" --path {gem_path}"
        ctx.run(cmd, env=env)


# hardened-runtime needs to be set to False to build on MacOS < 10.13.6, as the -o runtime option is not supported.
@task(
    help={
        'skip-sign': "On macOS, use this option to build an unsigned package if you don't have Datadog's developer keys.",
        'hardened-runtime': "On macOS, use this option to enforce the hardened runtime setting, adding '-o runtime' to all codesign commands",
    }
)
def omnibus_build(
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
    nikos_path=None,
    go_mod_cache=None,
):
    """
    Build the Agent packages with Omnibus Installer.
    """
    flavor = AgentFlavor[flavor]
    deps_elapsed = None
    bundle_elapsed = None
    omnibus_elapsed = None
    if not skip_deps:
        deps_start = datetime.datetime.now()
        deps(ctx)
        deps_end = datetime.datetime.now()
        deps_elapsed = deps_end - deps_start

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
        nikos_path=nikos_path,
        go_mod_cache=go_mod_cache,
        flavor=flavor,
    )

    target_project = "agent"
    if flavor.is_iot():
        target_project = "iot-agent"
    elif agent_binaries:
        target_project = "agent-binaries"

    bundle_start = datetime.datetime.now()
    bundle_install_omnibus(ctx, gem_path, env)
    bundle_done = datetime.datetime.now()
    bundle_elapsed = bundle_done - bundle_start

    omnibus_start = datetime.datetime.now()
    omnibus_run_task(
        ctx=ctx,
        task="build",
        target_project=target_project,
        base_dir=base_dir,
        env=env,
        omnibus_s3_cache=omnibus_s3_cache,
        log_level=log_level,
    )
    omnibus_done = datetime.datetime.now()
    omnibus_elapsed = omnibus_done - omnibus_start

    print("Build component timing:")
    if not skip_deps:
        print(f"Deps:    {deps_elapsed}")
    print(f"Bundle:  {bundle_elapsed}")
    print(f"Omnibus: {omnibus_elapsed}")


@task
def build_dep_tree(ctx, git_ref=""):
    """
    Generates a file representing the Golang dependency tree in the current
    directory. Use the "--git-ref=X" argument to specify which tag you would like
    to target otherwise current repo state will be used.
    """
    saved_branch = None
    if git_ref:
        print(f"Tag {git_ref} specified. Checking out the branch...")

        result = ctx.run("git rev-parse --abbrev-ref HEAD", hide='stdout')
        saved_branch = result.stdout

        ctx.run(f"git checkout {git_ref}")
    else:
        print("No tag specified. Using the current state of repository.")

    try:
        ctx.run("go run tools/dep_tree_resolver/go_deps.go")
    finally:
        if saved_branch:
            ctx.run(f"git checkout {saved_branch}", hide='stdout')


@task
def omnibus_manifest(
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


@task
def check_supports_python_version(_, check_dir, python):
    """
    Check if a Python project states support for a given major Python version.
    """
    import toml
    from packaging.specifiers import SpecifierSet

    if python not in ['2', '3']:
        raise Exit("invalid Python version", code=2)

    project_file = os.path.join(check_dir, 'pyproject.toml')
    setup_file = os.path.join(check_dir, 'setup.py')
    if os.path.isfile(project_file):
        with open(project_file, 'r') as f:
            data = toml.loads(f.read())

        project_metadata = data['project']
        if 'requires-python' not in project_metadata:
            print('True', end='')
            return

        specifier = SpecifierSet(project_metadata['requires-python'])
        # It might be e.g. `>=3.8` which would not immediatelly contain `3`
        for minor_version in range(100):
            if specifier.contains(f'{python}.{minor_version}'):
                print('True', end='')
                return
        else:
            print('False', end='')
    elif os.path.isfile(setup_file):
        with open(setup_file, 'r') as f:
            tree = ast.parse(f.read(), filename=setup_file)

        prefix = f'Programming Language :: Python :: {python}'
        for node in ast.walk(tree):
            if isinstance(node, ast.keyword) and node.arg == 'classifiers':
                classifiers = ast.literal_eval(node.value)
                print(any(cls.startswith(prefix) for cls in classifiers), end='')
                return
        else:
            print('False', end='')
    else:
        raise Exit('not a Python project', code=1)


@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/agent folder
    print("Remove agent binary folder")
    ctx.run("rm -rf ./bin/agent")

    print("Cleaning rtloader")
    rtloader_clean(ctx)


@task
def version(ctx, url_safe=False, omnibus_format=False, git_sha_length=7, major_version='7'):
    """
    Get the agent version.
    url_safe: get the version that is able to be addressed as a url
    omnibus_format: performs the same transformations omnibus does on version names to
                    get the exact same string that's used in package names
    git_sha_length: different versions of git have a different short sha length,
                    use this to explicitly set the version
                    (the windows builder and the default ubuntu version have such an incompatibility)
    """
    version = get_version(
        ctx,
        include_git=True,
        url_safe=url_safe,
        git_sha_length=git_sha_length,
        major_version=major_version,
        include_pipeline_id=True,
    )
    if omnibus_format:
        # See: https://github.com/DataDog/omnibus-ruby/blob/datadog-5.5.0/lib/omnibus/packagers/deb.rb#L599
        # In theory we'd need to have one format for each package type (deb, rpm, msi, pkg).
        # However, there are a few things that allow us in practice to have only one variable for everything:
        # - the deb and rpm safe version formats are identical (the only difference is an additional rule on Wind River Linux, which doesn't apply to us).
        #   Moreover, of the two rules, we actually really only use the first one (because we always use inv agent.version --url-safe).
        # - the msi version name uses the raw version string. The only difference with the deb / rpm versions
        #   is therefore that dashes are replaced by tildes. We're already doing the reverse operation in agent-release-management
        #   to get the correct msi name.
        # - the pkg version name uses the raw version + a variation of the second rule (where a dash is used in place of an underscore).
        #   Once again, replacing tildes by dashes (+ replacing underscore by dashes if we ever end up using the second rule for some reason)
        #   in agent-release-management is enough. We're already replacing tildes by dashes in agent-release-management.
        # TODO: investigate if having one format per package type in the agent.version method makes more sense.
        version = re.sub('-', '~', version)
        version = re.sub(r'[^a-zA-Z0-9\.\+\:\~]+', '_', version)
    print(version)


@task
def get_integrations_from_cache(ctx, python, bucket, branch, integrations_dir, target_dir, integrations, awscli="aws"):
    """
    Get cached integration wheels for given integrations.
    python: Python version to retrieve integrations for
    bucket: S3 bucket to retrieve integration wheels from
    branch: namespace in the bucket to get the integration wheels from
    integrations_dir: directory with Git repository of integrations
    target_dir: local directory to put integration wheels to
    integrations: comma-separated names of the integrations to try to retrieve from cache
    awscli: AWS CLI executable to call
    """
    integrations_hashes = {}
    for integration in integrations.strip().split(","):
        integration_path = os.path.join(integrations_dir, integration)
        if not os.path.exists(integration_path):
            raise Exit(f"Integration {integration} given, but doesn't exist in {integrations_dir}", code=2)
        last_commit = ctx.run(
            LAST_DIRECTORY_COMMIT_PATTERN.format(integrations_dir=integrations_dir, integration=integration),
            hide="both",
            echo=False,
        )
        integrations_hashes[integration] = last_commit.stdout.strip()

    print(f"Trying to retrieve {len(integrations_hashes)} integration wheels from cache")
    # On windows, maximum length of a command line call is 8191 characters, therefore
    # we do multiple syncs that fit within that limit (we use 8100 as a nice round number
    # and just to make sure we don't do any of-by-one errors that would break this).
    # WINDOWS NOTES: on Windows, the awscli is usually in program files, so we have to wrap the
    # executable in quotes; also we have to not put the * in quotes, as there's no
    # expansion on it, unlike on Linux
    exclude_wildcard = "*" if platform.system().lower() == "windows" else "'*'"
    sync_command_prefix = (
        f"\"{awscli}\" s3 sync s3://{bucket} {target_dir} --no-sign-request --exclude {exclude_wildcard}"
    )
    sync_commands = [[[sync_command_prefix], len(sync_command_prefix)]]
    for integration, hash in integrations_hashes.items():
        include_arg = " --include " + CACHED_WHEEL_FULL_PATH_PATTERN.format(
            hash=hash,
            integration=integration,
            python_version=python,
            branch=branch,
        )
        if len(include_arg) + sync_commands[-1][1] > 8100:
            sync_commands.append([[sync_command_prefix], len(sync_command_prefix)])
        sync_commands[-1][0].append(include_arg)
        sync_commands[-1][1] += len(include_arg)

    for sync_command in sync_commands:
        ctx.run("".join(sync_command[0]))

    found = []
    # move all wheel files directly to the target_dir, so they're easy to find/work with in Omnibus
    for integration in sorted(integrations_hashes):
        hash = integrations_hashes[integration]
        original_path_glob = os.path.join(
            target_dir,
            CACHED_WHEEL_FULL_PATH_PATTERN.format(
                hash=hash,
                integration=integration,
                python_version=python,
                branch=branch,
            ),
        )
        files_matched = glob.glob(original_path_glob)
        if len(files_matched) == 0:
            continue
        elif len(files_matched) > 1:
            raise Exit(
                f"More than 1 wheel for integration {integration} matched by {original_path_glob}: {files_matched}"
            )
        wheel_path = files_matched[0]
        print(f"Found cached wheel for integration {integration}")
        shutil.move(wheel_path, target_dir)
        found.append(f"datadog_{integration}")

    print(f"Found {len(found)} cached integration wheels")
    with open(os.path.join(target_dir, "found.txt"), "w") as f:
        f.write('\n'.join(found))


@task
def upload_integration_to_cache(ctx, python, bucket, branch, integrations_dir, build_dir, integration, awscli="aws"):
    """
    Upload a built integration wheel for given integration.
    python: Python version the integration is built for
    bucket: S3 bucket to upload the integration wheel to
    branch: namespace in the bucket to upload the integration wheels to
    integrations_dir: directory with Git repository of integrations
    build_dir: directory containing the built integration wheel
    integration: name of the integration being cached
    awscli: AWS CLI executable to call
    """
    matching_glob = os.path.join(build_dir, CACHED_WHEEL_FILENAME_PATTERN.format(integration=integration))
    files_matched = glob.glob(matching_glob)
    if len(files_matched) == 0:
        raise Exit(f"No wheel for integration {integration} found in {build_dir}")
    elif len(files_matched) > 1:
        raise Exit(f"More than 1 wheel for integration {integration} matched by {matching_glob}: {files_matched}")

    wheel_path = files_matched[0]

    last_commit = ctx.run(
        LAST_DIRECTORY_COMMIT_PATTERN.format(integrations_dir=integrations_dir, integration=integration),
        hide="both",
        echo=False,
    )
    hash = last_commit.stdout.strip()

    target_name = CACHED_WHEEL_DIRECTORY_PATTERN.format(
        hash=hash, python_version=python, branch=branch
    ) + os.path.basename(wheel_path)
    print(f"Caching wheel {target_name}")
    # NOTE: on Windows, the awscli is usually in program files, so we have the executable
    ctx.run(f"\"{awscli}\" s3 cp {wheel_path} s3://{bucket}/{target_name} --acl public-read")
