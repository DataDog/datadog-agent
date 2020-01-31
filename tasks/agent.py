"""
Agent namespaced tasks
"""
from __future__ import print_function
import datetime
import glob
import os
import shutil
import sys
import platform
from distutils.dir_util import copy_tree

import invoke
from invoke import task
from invoke.exceptions import Exit, ParseError

from .utils import bin_name, get_build_flags, get_version_numeric_only, load_release_versions, get_version, has_both_python, get_win_py_runtime_var
from .utils import REPO_PATH
from .build_tags import get_build_tags, get_default_build_tags, LINUX_ONLY_TAGS, REDHAT_AND_DEBIAN_ONLY_TAGS, REDHAT_AND_DEBIAN_DIST
from .go import deps
from .docker import pull_base_images
from .ssm import get_signing_cert, get_pfx_pass
from .rtloader import build as rtloader_build
from .rtloader import install as rtloader_install
from .rtloader import clean as rtloader_clean

# constants
BIN_PATH = os.path.join(".", "bin", "agent")
AGENT_TAG = "datadog/agent:master"
DEFAULT_BUILD_TAGS = [
    "apm",
    "process",
    "consul",
    "containerd",
    "python",
    "cri",
    "docker",
    "ec2",
    "etcd",
    "gce",
    "jmx",
    "kubeapiserver",
    "kubelet",
    "log",
    "netcgo",
    "systemd",
    "process",
    "zk",
    "zlib",
    "secrets",
]

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
    "systemd",
    "uptime",
    "winproc",
]

PUPPY_CORECHECKS = [
    "cpu",
    "disk",
    "io",
    "load",
    "memory",
    "network",
    "ntp",
    "uptime",
]


@task
def build(ctx, rebuild=False, race=False, build_include=None, build_exclude=None,
          puppy=False, development=True, precompile_only=False, skip_assets=False,
          embedded_path=None, rtloader_root=None, python_home_2=None, python_home_3=None,
          major_version='7', python_runtimes='3', arch='x64', exclude_rtloader=False):
    """
    Build the agent. If the bits to include in the build are not specified,
    the values from `invoke.yaml` will be used.

    Example invokation:
        inv agent.build --build-exclude=systemd
    """

    if not exclude_rtloader and not puppy:
        rtloader_build(ctx, python_runtimes=python_runtimes)
        rtloader_install(ctx)
    build_include = DEFAULT_BUILD_TAGS if build_include is None else build_include.split(",")
    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    ldflags, gcflags, env = get_build_flags(ctx, embedded_path=embedded_path,
            rtloader_root=rtloader_root, python_home_2=python_home_2, python_home_3=python_home_3,
            major_version=major_version, python_runtimes=python_runtimes, arch=arch)

    if not sys.platform.startswith('linux'):
        for ex in LINUX_ONLY_TAGS:
            if ex not in build_exclude:
                build_exclude.append(ex)

    # remove all tags that are only available on debian distributions
    distname = platform.linux_distribution()[0].lower()
    if distname not in REDHAT_AND_DEBIAN_DIST:
        for ex in REDHAT_AND_DEBIAN_ONLY_TAGS:
            if ex not in build_exclude:
                build_exclude.append(ex)

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
            build_arch=arch
        )
        command += "-i cmd/agent/agent.rc -O coff -o cmd/agent/rsrc.syso"
        ctx.run(command, env=env)

    if puppy:
        # Puppy mode overrides whatever passed through `--build-exclude` and `--build-include`
        build_tags = get_default_build_tags(puppy=True)
    else:
        build_tags = get_build_tags(build_include, build_exclude)

    cmd = "go build {race_opt} {build_type} -tags \"{go_build_tags}\" "

    cmd += "-o {agent_bin} -gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/agent"
    args = {
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else ("-i" if precompile_only else ""),
        "go_build_tags": " ".join(build_tags),
        "agent_bin": os.path.join(BIN_PATH, bin_name("agent", android=False)),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args), env=env)

    # Remove cross-compiling bits to render config
    env.update({
        "GOOS": "",
        "GOARCH": "",
    })

    # Render the Agent configuration file template
    cmd = "go run {go_file} {build_type} {template_file} {output_file}"

    args = {
        "go_file": "./pkg/config/render_config.go",
        "build_type": "agent-py2py3" if has_both_python(python_runtimes) else "agent-py3",
        "template_file": "./pkg/config/config_template.yaml",
        "output_file": "./cmd/agent/dist/datadog.yaml",
    }

    ctx.run(cmd.format(**args), env=env)

    # On Linux and MacOS, render the system-probe configuration file template
    if sys.platform != 'win32':
        cmd = "go run ./pkg/config/render_config.go system-probe ./pkg/config/config_template.yaml ./cmd/agent/dist/system-probe.yaml"
        ctx.run(cmd, env=env)

    if not skip_assets:
        refresh_assets(ctx, build_tags, development=development, puppy=puppy)

@task
def refresh_assets(ctx, build_tags, development=True, puppy=False):
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
    if not puppy:
        shutil.copy("./cmd/agent/dist/dd-agent", os.path.join(dist_folder, "dd-agent"))
        # copy the dd-agent placeholder to the bin folder
        bin_ddagent = os.path.join(BIN_PATH, "dd-agent")
        shutil.move(os.path.join(dist_folder, "dd-agent"), bin_ddagent)

    # System probe not supported on windows
    if sys.platform.startswith('linux'):
      shutil.copy("./cmd/agent/dist/system-probe.yaml", os.path.join(dist_folder, "system-probe.yaml"))
    shutil.copy("./cmd/agent/dist/datadog.yaml", os.path.join(dist_folder, "datadog.yaml"))

    for check in AGENT_CORECHECKS if not puppy else PUPPY_CORECHECKS:
        check_dir = os.path.join(dist_folder, "conf.d/{}.d/".format(check))
        copy_tree("./cmd/agent/dist/conf.d/{}.d/".format(check), check_dir)
    if "apm" in build_tags:
        shutil.copy("./cmd/agent/dist/conf.d/apm.yaml.default", os.path.join(dist_folder, "conf.d/apm.yaml.default"))
    if "process" in build_tags:
        shutil.copy("./cmd/agent/dist/conf.d/process_agent.yaml.default", os.path.join(dist_folder, "conf.d/process_agent.yaml.default"))

    copy_tree("./pkg/status/dist/", dist_folder)
    copy_tree("./cmd/agent/gui/views", os.path.join(dist_folder, "views"))
    if development:
        copy_tree("./dev/dist/", dist_folder)


@task
def run(ctx, rebuild=False, race=False, build_include=None, build_exclude=None,
        puppy=False, skip_build=False):
    """
    Execute the agent binary.

    By default it builds the agent before executing it, unless --skip-build was
    passed. It accepts the same set of options as agent.build.
    """
    if not skip_build:
        build(ctx, rebuild, race, build_include, build_exclude, puppy)

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
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False):
    """
    Run integration tests for the Agent
    """
    if install_deps:
        deps(ctx)

    test_args = {
        "go_build_tags": " ".join(get_default_build_tags()),
        "race_opt": "-race" if race else "",
        "exec_opts": "",
    }

    if remote_docker:
        test_args["exec_opts"] = "-exec \"inv docker.dockerize-test\""

    go_cmd = 'go test {race_opt} -tags "{go_build_tags}" {exec_opts}'.format(**test_args)

    prefixes = [
        "./test/integration/config_providers/...",
        "./test/integration/corechecks/...",
        "./test/integration/listeners/...",
        "./test/integration/util/kubelet/...",
    ]

    for prefix in prefixes:
        ctx.run("{} {}".format(go_cmd, prefix))


@task(help={'skip-sign': "On macOS, use this option to build an unsigned package if you don't have Datadog's developer keys."})
def omnibus_build(ctx, puppy=False, cf_windows=False, log_level="info", base_dir=None, gem_path=None,
                  skip_deps=False, skip_sign=False, release_version="nightly", major_version='7',
                  python_runtimes='3', omnibus_s3_cache=False):
    """
    Build the Agent packages with Omnibus Installer.
    """
    deps_elapsed = None
    bundle_elapsed = None
    omnibus_elapsed = None
    if not skip_deps:
        deps_start = datetime.datetime.now()
        deps(ctx, no_checks=True)  # no_checks since the omnibus build installs checks with a dedicated software def
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
        if puppy:
            target_project = "puppy"
        elif cf_windows:
            target_project = "cf-windows"

        omnibus = "bundle exec omnibus.bat" if sys.platform == 'win32' else "bundle exec omnibus"
        cmd = "{omnibus} build {project_name} --log-level={log_level} {populate_s3_cache} {overrides}"
        args = {
            "omnibus": omnibus,
            "project_name": target_project,
            "log_level": log_level,
            "overrides": overrides_cmd,
            "populate_s3_cache": ""
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

            if omnibus_s3_cache:
                args['populate_s3_cache'] = " --populate-s3-cache "
            if skip_sign:
                env['SKIP_SIGN_MAC'] = 'true'

            env['PACKAGE_VERSION'] = get_version(ctx, include_git=True, url_safe=True, major_version=major_version, env=env)
            env['MAJOR_VERSION'] = major_version
            env['PY_RUNTIMES'] = python_runtimes
            omnibus_start = datetime.datetime.now()
            ctx.run(cmd.format(**args), env=env)
            omnibus_done = datetime.datetime.now()
            omnibus_elapsed = omnibus_done - omnibus_start


        except Exception as e:
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
    print(get_version(ctx, include_git=True, url_safe=url_safe, git_sha_length=git_sha_length, major_version=major_version))
