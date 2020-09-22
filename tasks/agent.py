"""
Agent namespaced tasks
"""
from __future__ import print_function

import datetime
import glob
import os
import shutil
import sys
from distutils.dir_util import copy_tree

from invoke import task
from invoke.exceptions import Exit, ParseError

from .build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from .docker import pull_base_images
from .go import deps, generate
from .rtloader import clean as rtloader_clean
from .rtloader import install as rtloader_install
from .rtloader import make as rtloader_make
from .ssm import get_pfx_pass, get_signing_cert
from .utils import (
    REPO_PATH,
    bin_name,
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
    "containerd",
    "cpu",
    "cri",
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
    "winproc",
]

IOT_AGENT_CORECHECKS = [
    "cpu",
    "disk",
    "io",
    "jetson",
    "load",
    "memory",
    "network",
    "ntp",
    "uptime",
    "systemd",
]


@task
def build(
    ctx,
    rebuild=False,
    race=False,
    build_include=None,
    build_exclude=None,
    iot=False,
    development=True,
    precompile_only=False,
    skip_assets=False,
    embedded_path=None,
    rtloader_root=None,
    python_home_2=None,
    python_home_3=None,
    major_version='7',
    python_runtimes='3',
    arch='x64',
    exclude_rtloader=False,
    go_mod="vendor",
    windows_sysprobe=False,
):
    """
    Build the agent. If the bits to include in the build are not specified,
    the values from `invoke.yaml` will be used.

    Example invokation:
        inv agent.build --build-exclude=systemd
    """

    if not exclude_rtloader and not iot:
        rtloader_make(ctx, python_runtimes=python_runtimes)
        rtloader_install(ctx)

    ldflags, gcflags, env = get_build_flags(
        ctx,
        embedded_path=embedded_path,
        rtloader_root=rtloader_root,
        python_home_2=python_home_2,
        python_home_3=python_home_3,
        major_version=major_version,
        python_runtimes=python_runtimes,
        arch=arch,
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
        ver = get_version_numeric_only(ctx, env, major_version=major_version)
        build_maj, build_min, build_patch = ver.split(".")

        command = "windmc --target {target_arch} -r cmd/agent cmd/agent/agentmsg.mc ".format(target_arch=windres_target)
        ctx.run(command, env=env)

        command = "windres --target {target_arch} --define {py_runtime_var}=1 --define MAJ_VER={build_maj} --define MIN_VER={build_min} --define PATCH_VER={build_patch} --define BUILD_ARCH_{build_arch}=1".format(
            py_runtime_var=py_runtime_var,
            build_maj=build_maj,
            build_min=build_min,
            build_patch=build_patch,
            target_arch=windres_target,
            build_arch=arch,
        )
        command += "-i cmd/agent/agent.rc -O coff -o cmd/agent/rsrc.syso"
        ctx.run(command, env=env)

    if iot:
        # Iot mode overrides whatever passed through `--build-exclude` and `--build-include`
        build_tags = get_default_build_tags(build="iot", arch=arch)
    else:
        build_include = (
            get_default_build_tags(build="agent", arch=arch)
            if build_include is None
            else filter_incompatible_tags(build_include.split(","), arch=arch)
        )
        build_exclude = [] if build_exclude is None else build_exclude.split(",")
        build_tags = get_build_tags(build_include, build_exclude)

    # Generating go source from templates by running go generate on ./pkg/status
    generate(ctx)

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
        "flavor": "iot-agent" if iot else "agent",
    }
    ctx.run(cmd.format(**args), env=env)

    # Remove cross-compiling bits to render config
    env.update(
        {"GOOS": "", "GOARCH": "",}
    )

    # Render the Agent configuration file template
    cmd = "go run {go_file} {build_type} {template_file} {output_file}"

    build_type = "agent-py3"
    if iot:
        build_type = "iot-agent"
    elif has_both_python(python_runtimes):
        build_type = "agent-py2py3"

    args = {
        "go_file": "./pkg/config/render_config.go",
        "build_type": build_type,
        "template_file": "./pkg/config/config_template.yaml",
        "output_file": "./cmd/agent/dist/datadog.yaml",
    }

    ctx.run(cmd.format(**args), env=env)

    # On Linux and MacOS, render the system-probe configuration file template
    if sys.platform != 'win32' or windows_sysprobe:
        cmd = "go run ./pkg/config/render_config.go system-probe ./pkg/config/config_template.yaml ./cmd/agent/dist/system-probe.yaml"
        ctx.run(cmd, env=env)

    if not skip_assets:
        refresh_assets(ctx, build_tags, development=development, iot=iot, windows_sysprobe=windows_sysprobe)


@task
def refresh_assets(ctx, build_tags, development=True, iot=False, windows_sysprobe=False):
    """
    Clean up and refresh Collector's assets and config files
    """
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
    if not iot:
        shutil.copy("./cmd/agent/dist/dd-agent", os.path.join(dist_folder, "dd-agent"))
        # copy the dd-agent placeholder to the bin folder
        bin_ddagent = os.path.join(BIN_PATH, "dd-agent")
        shutil.move(os.path.join(dist_folder, "dd-agent"), bin_ddagent)

    # System probe not supported on windows
    if sys.platform.startswith('linux') or windows_sysprobe:
        shutil.copy("./cmd/agent/dist/system-probe.yaml", os.path.join(dist_folder, "system-probe.yaml"))
    shutil.copy("./cmd/agent/dist/datadog.yaml", os.path.join(dist_folder, "datadog.yaml"))

    for check in AGENT_CORECHECKS if not iot else IOT_AGENT_CORECHECKS:
        check_dir = os.path.join(dist_folder, "conf.d/{}.d/".format(check))
        copy_tree("./cmd/agent/dist/conf.d/{}.d/".format(check), check_dir)
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
def run(ctx, rebuild=False, race=False, build_include=None, build_exclude=None, iot=False, skip_build=False):
    """
    Execute the agent binary.

    By default it builds the agent before executing it, unless --skip-build was
    passed. It accepts the same set of options as agent.build.
    """
    if not skip_build:
        build(ctx, rebuild, race, build_include, build_exclude, iot)

    ctx.run(os.path.join(BIN_PATH, bin_name("agent")))


@task
def system_tests(ctx):
    """
    Run the system testsuite.
    """
    pass


@task
def image_build(ctx, arch='amd64', base_dir="omnibus", python_version="2", skip_tests=False):
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
    deb_glob = 'datadog-agent*_{}.deb'.format(arch)
    dockerfile_path = "{}/{}/Dockerfile".format(build_context, arch)
    list_of_files = glob.glob(os.path.join(pkg_dir, deb_glob))
    # get the last debian package built
    if not list_of_files:
        print("No debian package build found in {}".format(pkg_dir))
        print("See agent.omnibus-build")
        raise Exit(code=1)
    latest_file = max(list_of_files, key=os.path.getctime)
    shutil.copy2(latest_file, build_context)

    # Pull base image with content trust enabled
    pull_base_images(ctx, dockerfile_path, signed_pull=True)
    common_build_opts = "-t {} -f {}".format(AGENT_TAG, dockerfile_path)
    if python_version not in BOTH_VERSIONS:
        common_build_opts = "{} --build-arg PYTHON_VERSION={}".format(common_build_opts, python_version)

    # Build with the testing target
    if not skip_tests:
        ctx.run("docker build {} --target testing {}".format(common_build_opts, build_context))

    # Build with the release target
    ctx.run("docker build {} --target release {}".format(common_build_opts, build_context))
    ctx.run("rm {}/{}".format(build_context, deb_glob))


@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False, go_mod="vendor", arch="x64"):
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
        test_args["exec_opts"] = "-exec \"{}/test/integration/dockerize_tests.sh\"".format(os.getcwd())

    go_cmd = 'go test -mod={go_mod} {race_opt} -tags "{go_build_tags}" {exec_opts}'.format(**test_args)

    prefixes = [
        "./test/integration/config_providers/...",
        "./test/integration/corechecks/...",
        "./test/integration/listeners/...",
        "./test/integration/util/kubelet/...",
    ]

    for prefix in prefixes:
        ctx.run("{} {}".format(go_cmd, prefix))


# hardened-runtime needs to be set to False to build on MacOS < 10.13.6, as the -o runtime option is not supported.
@task(
    help={
        'skip-sign': "On macOS, use this option to build an unsigned package if you don't have Datadog's developer keys.",
        'hardened-runtime': "On macOS, use this option to enforce the hardened runtime setting, adding '-o runtime' to all codesign commands",
    }
)
def omnibus_build(
    ctx,
    iot=False,
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
    libbcc_tarball=None,
    with_bcc=True,
):
    """
    Build the Agent packages with Omnibus Installer.
    """
    deps_elapsed = None
    bundle_elapsed = None
    omnibus_elapsed = None
    if not skip_deps:
        deps_start = datetime.datetime.now()
        deps(ctx)
        deps_end = datetime.datetime.now()
        deps_elapsed = deps_end - deps_start

    # omnibus config overrides
    overrides = []

    # base dir (can be overridden through env vars, command line takes precedence)
    base_dir = base_dir or os.environ.get("OMNIBUS_BASE_DIR")
    if base_dir:
        overrides.append("base_dir:{}".format(base_dir))

    overrides_cmd = ""
    if overrides:
        overrides_cmd = "--override=" + " ".join(overrides)

    with ctx.cd("omnibus"):
        # make sure bundle install starts from a clean state
        try:
            os.remove("Gemfile.lock")
        except Exception:
            pass

        env = load_release_versions(ctx, release_version)

        cmd = "bundle install"
        if gem_path:
            cmd += " --path {}".format(gem_path)

        bundle_start = datetime.datetime.now()
        ctx.run(cmd, env=env)

        bundle_done = datetime.datetime.now()
        bundle_elapsed = bundle_done - bundle_start
        target_project = "agent"
        if iot:
            target_project = "iot-agent"
        elif agent_binaries:
            target_project = "agent-binaries"

        omnibus = "bundle exec omnibus"
        if sys.platform == 'win32':
            omnibus = "bundle exec omnibus.bat"
        elif sys.platform == 'darwin':
            # HACK: This is an ugly hack to fix another hack made by python3 on MacOS
            # The full explanation is available on this PR: https://github.com/DataDog/datadog-agent/pull/5010.
            omnibus = "unset __PYVENV_LAUNCHER__ && bundle exec omnibus"

        cmd = "{omnibus} build {project_name} --log-level={log_level} {populate_s3_cache} {overrides}"
        args = {
            "omnibus": omnibus,
            "project_name": target_project,
            "log_level": log_level,
            "overrides": overrides_cmd,
            "populate_s3_cache": "",
        }
        pfxfile = None
        try:
            if sys.platform == 'win32' and os.environ.get('SIGN_WINDOWS'):
                # get certificate and password from ssm
                pfxfile = get_signing_cert(ctx)
                pfxpass = get_pfx_pass(ctx)
                # hack for now.  Remove `sign_windows, and set sign_pfx`
                env['SIGN_PFX'] = "{}".format(pfxfile)
                env['SIGN_PFX_PW'] = "{}".format(pfxpass)

            if sys.platform == 'darwin':
                # Target MacOS 10.12
                env['MACOSX_DEPLOYMENT_TARGET'] = '10.12'

            if omnibus_s3_cache:
                args['populate_s3_cache'] = " --populate-s3-cache "
            if skip_sign:
                env['SKIP_SIGN_MAC'] = 'true'
            if hardened_runtime:
                env['HARDENED_RUNTIME_MAC'] = 'true'

            env['PACKAGE_VERSION'] = get_version(
                ctx, include_git=True, url_safe=True, major_version=major_version, env=env
            )
            env['MAJOR_VERSION'] = major_version
            env['PY_RUNTIMES'] = python_runtimes
            if with_bcc:
                env['WITH_BCC'] = 'true'
            if system_probe_bin is not None:
                env['SYSTEM_PROBE_BIN'] = system_probe_bin
            if libbcc_tarball is not None:
                env['LIBBCC_TARBALL'] = libbcc_tarball
            omnibus_start = datetime.datetime.now()
            ctx.run(cmd.format(**args), env=env)
            omnibus_done = datetime.datetime.now()
            omnibus_elapsed = omnibus_done - omnibus_start

        except Exception:
            if pfxfile:
                os.remove(pfxfile)
            raise

        if pfxfile:
            os.remove(pfxfile)

        print("Build compoonent timing:")
        if not skip_deps:
            print("Deps:    {}".format(deps_elapsed))
        print("Bundle:  {}".format(bundle_elapsed))
        print("Omnibus: {}".format(omnibus_elapsed))


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
def version(ctx, url_safe=False, git_sha_length=7, major_version='7'):
    """
    Get the agent version.
    url_safe: get the version that is able to be addressed as a url
    git_sha_length: different versions of git have a different short sha length,
                    use this to explicitly set the version
                    (the windows builder and the default ubuntu version have such an incompatibility)
    """
    print(
        get_version(
            ctx, include_git=True, url_safe=url_safe, git_sha_length=git_sha_length, major_version=major_version
        )
    )
