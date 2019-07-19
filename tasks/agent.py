"""
Agent namespaced tasks
"""
from __future__ import print_function
import glob
import os
import shutil
import sys
import platform
from distutils.dir_util import copy_tree

import invoke
from invoke import task
from invoke.exceptions import Exit

from .utils import bin_name, get_build_flags, get_version_numeric_only, load_release_versions, get_version
from .utils import REPO_PATH
from .build_tags import get_build_tags, get_default_build_tags, LINUX_ONLY_TAGS, REDHAT_AND_DEBIAN_ONLY_TAGS, REDHAT_AND_DEBIAN_DIST
from .go import deps

# constants
BIN_PATH = os.path.join(".", "bin", "agent")
AGENT_TAG = "datadog/agent:master"
DEFAULT_BUILD_TAGS = [
    "apm",
    "consul",
    "containerd",
    "cpython",
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
    "snmp",
    "zk",
    "zlib",
]

AGENT_CORECHECKS = [
    "cpu",
    "cri",
    "containerd",
    "docker",
    "file_handle",
    "go_expvar",
    "io",
    "jmx",
    "kubernetes_apiserver",
    "load",
    "memory",
    "ntp",
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


def do_go_rename(ctx, rename, at):
    ctx.run("gofmt -l -w -r {} {}".format(rename, at))


def do_sed_rename(ctx, rename, at):
    ctx.run("sed -i '{}' {}".format(rename, at))


@task
def apply_branding(ctx):
    """
    Apply stackstate branding
    """
    # Config
    do_go_rename(ctx, '"\\"dd_url\\" -> \\"sts_url\\""', "./pkg/config")
    do_go_rename(ctx, '"\\"https://app.datadoghq.com\\" -> \\"http://localhost:7077\\""', "./pkg/config")
    do_go_rename(ctx, '"\\"DD_PROXY_HTTP\\" -> \\"STS_PROXY_HTTP\\""', "./pkg/config")
    do_go_rename(ctx, '"\\"DD_PROXY_HTTPS\\" -> \\"STS_PROXY_HTTPS\\""', "./pkg/config")
    do_go_rename(ctx, '"\\"DD_PROXY_NO_PROXY\\" -> \\"STS_PROXY_NO_PROXY\\""', "./pkg/config")
    do_go_rename(ctx, '"\\"DOCKER_DD_AGENT\\" -> \\"DOCKER_STS_AGENT\\""', "./pkg/config")
    do_go_rename(ctx, '"\\"DD\\" -> \\"STS\\""', "./pkg/config")
    do_go_rename(ctx, '"\\"datadog\\" -> \\"stackstate\\""', "./pkg/config")
    do_go_rename(ctx, '"\\"/etc/datadog-agent/conf.d\\" -> \\"/etc/stackstate-agent/conf.d\\""', "./pkg/config")
    do_go_rename(ctx, '"\\"/etc/datadog-agent/checks.d\\" -> \\"/etc/stackstate-agent/checks.d\\""', "./pkg/config")
    do_go_rename(ctx, '"\\"/opt/datadog-agent/run\\" -> \\"/op/stackstate-agent/run\\""', "./pkg/config")

    # Trace agent
    do_go_rename(ctx, '"\\"DD_PROXY_HTTPS\\" -> \\"STS_PROXY_HTTPS\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_CONNECTION_LIMIT\\" -> \\"STS_CONNECTION_LIMIT\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_APM_CONNECTION_LIMIT\\" -> \\"STS_APM_CONNECTION_LIMIT\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_RECEIVER_PORT\\" -> \\"STS_RECEIVER_PORT\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_APM_RECEIVER_PORT\\" -> \\"STS_APM_RECEIVER_PORT\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_MAX_EPS\\" -> \\"STS_MAX_EPS\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_MAX_TPS\\" -> \\"STS_MAX_TPS\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_APM_MAX_TPS\\" -> \\"STS_APM_MAX_TPS\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_IGNORE_RESOURCE\\" -> \\"STS_IGNORE_RESOURCE\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_APM_IGNORE_RESOURCES\\" -> \\"STS_APM_IGNORE_RESOURCES\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_API_KEY\\" -> \\"STS_API_KEY\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_SITE\\" -> \\"STS_SITE\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_APM_ENABLED\\" -> \\"STS_APM_ENABLED\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_APM_DD_URL\\" -> \\"STS_APM_URL\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_HOSTNAME\\" -> \\"STS_HOSTNAME\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_BIND_HOST\\" -> \\"STS_BIND_HOST\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_DOGSTATSD_PORT\\" -> \\"STS_DOGSTATSD_PORT\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_APM_NON_LOCAL_TRAFFIC\\" -> \\"STS_APM_NON_LOCAL_TRAFFIC\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_LOG_LEVEL\\" -> \\"STS_LOG_LEVEL\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_APM_ANALYZED_SPANS\\" -> \\"STS_APM_ANALYZED_SPANS\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_APM_MAX_EPS\\" -> \\"STS_APM_MAX_EPS\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_APM_ENV\\" -> \\"STS_APM_ENV\\""', "./pkg/trace")
    do_go_rename(ctx, '"\\"DD_APM_MAX_MEMORY\\" -> \\"STS_APM_MAX_MEMORY\\""', "./pkg/trace")

    do_go_rename(ctx, '"\\"/var/log/datadog/trace-agent.log\\" -> \\"/var/log/stackstate-agent/trace-agent.log\\""', "./pkg/trace/config/")
    do_go_rename(ctx, '"\\"/opt/datadog-agent/embedded/bin/python\\" -> \\"/opt/stackstate-agent/embedded/bin/python\\""', "./pkg/trace/config/")
    do_go_rename(ctx, '"\\"PYTHONPATH=/opt/datadog-agent/agent\\" -> \\"PYTHONPATH=/opt/stackstate-agent/agent\\""', "./pkg/trace/config/")
    do_go_rename(ctx, '"\\"/var/log/datadog/agent.log\\" -> \\"/var/log/stackstate-agent/agent.log\\""', "./pkg/trace/config/")
    do_go_rename(ctx, '"\\"/opt/datadog-agent/bin/agent/agent\\" -> \\"/opt/stackstate-agent/bin/agent/agent\\""', "./pkg/trace/config/")
    do_go_rename(ctx, '"\\"/etc/dd-agent/datadog.conf\\" -> \\"/etc/sts-agent/stackstate.conf\\""', "./pkg/trace/config/")

    do_go_rename(ctx, '"\\"Datadog Trace Agent\\" -> \\"Stackstate Trace Agent\\""', "./pkg/trace/writer/")
    do_go_rename(ctx, '"\\"https://github.com/DataDog/datadog-trace-agent\\" -> \\"https://github.com/Stackvista/stackstate-trace-agent\\""', "./pkg/trace/writer/")

    # / Trace agent

    apm_dd_url_replace = 's/apm_dd_url/apm_sts_url/g'
    do_sed_rename(ctx, apm_dd_url_replace, "./pkg/trace/config/apply.go")
    do_sed_rename(ctx, apm_dd_url_replace, "./pkg/trace/config/env.go")

    do_sed_rename(ctx, 's/DD_APM_ENABLED/STS_APM_ENABLED/g', "./pkg/trace/agent/run.go")

    dd_agent_bin_replace = 's/dd_agent_bin/sts_agent_bin/g'
    do_sed_rename(ctx, dd_agent_bin_replace, "./pkg/trace/config/apply.go")

    DD_API_KEY_replace = 's/DD_API_KEY/STS_API_KEY/g'
    do_sed_rename(ctx, DD_API_KEY_replace, "./pkg/trace/config/config.go")

    DD_HOSTNAME_replace = 's/DD_HOSTNAME/STS_HOSTNAME/g'
    do_sed_rename(ctx, DD_HOSTNAME_replace, "./pkg/trace/config/config.go")

    # Defaults
    do_go_rename(ctx, '"\\"/etc/datadog-agent\\" -> \\"/etc/stackstate-agent\\""', "./cmd/agent/common")
    do_go_rename(ctx, '"\\"/var/log/datadog/agent.log\\" -> \\"/var/log/stackstate-agent/agent.log\\""', "./cmd/agent/common")
    do_go_rename(ctx, '"\\"/var/log/datadog/cluster-agent.log\\" -> \\"/var/log/stackstate-agent/cluster-agent.log\\""', "./cmd/agent/common")
    do_go_rename(ctx, '"\\"datadog.yaml\\" -> \\"stackstate.yaml\\""', "./cmd/agent")
    do_go_rename(ctx, '"\\"datadog.conf\\" -> \\"stackstate.conf\\""', "./cmd/agent")
    do_go_rename(ctx, '"\\"path to directory containing datadog.yaml\\" -> \\"path to directory containing stackstate.yaml\\""', "./cmd")
    do_go_rename(ctx, '"\\"unable to load Datadog config file: %s\\" -> \\"unable to load StackState config file: %s\\""', "./cmd/agent/common")
    do_go_rename(ctx, '"\\"Starting Datadog Agent v%v\\" -> \\"Starting StackState Agent v%v\\""', "./cmd/agent/app")

    camel_replace = 's/Data[dD]og/StackState/g'
    lower_replace = 's/datadog/stackstate/g'

    # Hardcoded checks and metrics
    do_sed_rename(ctx, lower_replace, "./pkg/aggregator/aggregator.go")

    # Windows defaults
    do_sed_rename(ctx, camel_replace, "./cmd/agent/agent.rc")
    do_sed_rename(ctx, camel_replace, "./cmd/agent/app/install_service_windows.go")
    do_sed_rename(ctx, lower_replace, "./cmd/agent/app/dependent_services_windows.go")
    # replace strings NOT containing certain pattern
    do_sed_rename(ctx, '/config/! s/Data[dD]og/StackState/g', "./cmd/agent/common/common_windows.go")
    do_sed_rename(ctx, lower_replace, "./cmd/agent/common/common_windows.go")
    do_sed_rename(ctx, 's/dd_url/sts_url/', "./cmd/agent/common/common_windows.go")
    do_sed_rename(ctx, lower_replace, "./cmd/dogstatsd/main_windows.go")
    do_sed_rename(ctx, camel_replace, "./pkg/config/config_windows.go")

    # Windows MSI installation
    do_sed_rename(ctx, camel_replace, "./omnibus/resources/agent/msi/cal/CustomAction.cpp")
    do_sed_rename(ctx, lower_replace, "./omnibus/resources/agent/msi/cal/CustomAction.cpp")
    do_sed_rename(ctx, camel_replace, "./omnibus/resources/agent/msi/cal/CustomAction.def")
    do_sed_rename(ctx, camel_replace, "./omnibus/resources/agent/msi/localization-en-us.wxl.erb")
    do_sed_rename(ctx, 's/"datadog\.yaml\.example"/"stackstate\.yaml\.example"/', "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, 's/datadoghq\.com/www\.stackstate\.com/', "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, camel_replace, "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, lower_replace, "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, 's/DATADOG/STACKSTATE/', "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, 's/dd_url/sts_url/', "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, 's/\[.*DD_URL\]/\[STS_URL\]/', "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, camel_replace, "./omnibus/resources/agent/msi/bundle.wxs.erb")
    do_sed_rename(ctx, 's/dd_logo_side\\.png/sts_logo_side\\.png/', "./omnibus/resources/agent/msi/bundle.wxs.erb")

    # Windows SysTray and GUI
    tray_replace = 's/ddtray/ststray/'
    do_sed_rename(ctx, lower_replace, "./cmd/systray/doservicecontrol.go")
    do_sed_rename(ctx, camel_replace, "./cmd/systray/systray.go")
    do_sed_rename(ctx, tray_replace, "./cmd/systray/systray.go")
    do_sed_rename(ctx, camel_replace, "./cmd/systray/systray.rc")
    do_sed_rename(ctx, tray_replace, "./cmd/systray/systray.rc")
    do_sed_rename(ctx, tray_replace, "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, tray_replace, "./tasks/systray.py")
    do_sed_rename(ctx, lower_replace, "./cmd/agent/gui/views/templates/index.tmpl")
    do_sed_rename(ctx, 's/"DataDog Agent 6"/"StackState Agent 2"/', "./cmd/agent/gui/views/templates/index.tmpl")
    do_sed_rename(ctx, camel_replace, "./cmd/agent/gui/views/templates/index.tmpl")
    do_sed_rename(ctx, camel_replace, "./cmd/agent/gui/views/private/js/javascript.js")


@task
def build(ctx, rebuild=False, race=False, build_include=None, build_exclude=None,
          puppy=False, use_embedded_libs=False, development=True, precompile_only=False,
          skip_assets=False, use_venv=False):
    """
    Build the agent. If the bits to include in the build are not specified,
    the values from `invoke.yaml` will be used.

    Example invokation:
        inv agent.build --build-exclude=snmp,systemd
    """

    build_include = DEFAULT_BUILD_TAGS if build_include is None else build_include.split(",")
    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    ldflags, gcflags, env = get_build_flags(ctx, use_embedded_libs=use_embedded_libs, use_venv=use_venv)

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
        # This generates the manifest resource. The manifest resource is necessary for
        # being able to load the ancient C-runtime that comes along with Python 2.7
        # command = "rsrc -arch amd64 -manifest cmd/agent/agent.exe.manifest -o cmd/agent/rsrc.syso"
        ver = get_version_numeric_only(ctx)
        build_maj, build_min, build_patch = ver.split(".")

        command = "windmc --target pe-x86-64 -r cmd/agent cmd/agent/agentmsg.mc "
        ctx.run(command, env=env)

        command = "windres --define MAJ_VER={build_maj} --define MIN_VER={build_min} --define PATCH_VER={build_patch} ".format(
            build_maj=build_maj,
            build_min=build_min,
            build_patch=build_patch
        )
        command += "-i cmd/agent/agent.rc --target=pe-x86-64 -O coff -o cmd/agent/rsrc.syso"
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

    # Render the configuration file template
    #
    # We need to remove cross compiling bits if any because go generate must
    # build and execute in the native platform
    env.update({
        "GOOS": "",
        "GOARCH": "",
    })
    cmd = "go generate {}/cmd/agent"
    ctx.run(cmd.format(REPO_PATH), env=env)

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

    if "cpython" in build_tags:
        copy_tree("./cmd/agent/dist/checks/", os.path.join(dist_folder, "checks"))
        copy_tree("./cmd/agent/dist/utils/", os.path.join(dist_folder, "utils"))
        shutil.copy("./cmd/agent/dist/config.py", os.path.join(dist_folder, "config.py"))
    if not puppy:
        shutil.copy("./cmd/agent/dist/dd-agent", os.path.join(dist_folder, "dd-agent"))
        # copy the dd-agent placeholder to the bin folder
        bin_ddagent = os.path.join(BIN_PATH, "sts-agent")
        shutil.move(os.path.join(dist_folder, "dd-agent"), bin_ddagent)

    # Network tracer not supported on windows
    if sys.platform.startswith('linux'):
      shutil.copy("./cmd/agent/dist/network-tracer.yaml", os.path.join(dist_folder, "network-tracer.yaml"))
    shutil.copy("./cmd/agent/dist/datadog.yaml", os.path.join(dist_folder, "datadog.yaml"))

    for check in AGENT_CORECHECKS if not puppy else PUPPY_CORECHECKS:
        check_dir = os.path.join(dist_folder, "conf.d/{}.d/".format(check))
        copy_tree("./cmd/agent/dist/conf.d/{}.d/".format(check), check_dir)
    if "apm" in build_tags:
        shutil.copy("./cmd/agent/dist/conf.d/apm.yaml.default", os.path.join(dist_folder, "conf.d/apm.yaml.default"))

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
def image_build(ctx, base_dir="omnibus"):
    """
    Build the docker image
    """
    base_dir = base_dir or os.environ.get("OMNIBUS_BASE_DIR")
    pkg_dir = os.path.join(base_dir, 'pkg')
    list_of_files = glob.glob(os.path.join(pkg_dir, 'datadog-agent*_amd64.deb'))
    # get the last debian package built
    if not list_of_files:
        print("No debian package build found in {}".format(pkg_dir))
        print("See agent.omnibus-build")
        raise Exit(code=1)
    latest_file = max(list_of_files, key=os.path.getctime)
    shutil.copy2(latest_file, "Dockerfiles/agent/")
    ctx.run("docker build -t {} Dockerfiles/agent".format(AGENT_TAG))
    ctx.run("rm Dockerfiles/agent/datadog-agent*_amd64.deb")


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
def omnibus_build(ctx, puppy=False, log_level="info", base_dir=None, gem_path=None,
                  skip_deps=False, skip_sign=False, omnibus_s3_cache=False):
    """
    Build the Agent packages with Omnibus Installer.
    """
    if not skip_deps:
        deps(ctx, no_checks=True)  # no_checks since the omnibus build installs checks with a dedicated software def

    apply_branding(ctx)

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
        env = load_release_versions(ctx)
        cmd = "bundle install"
        if gem_path:
            cmd += " --path {}".format(gem_path)
        ctx.run(cmd, env=env)

        omnibus = "bundle exec omnibus.bat" if sys.platform == 'win32' else "bundle exec omnibus"
        cmd = "{omnibus} build {project_name} --log-level={log_level} {populate_s3_cache} {overrides}"
        args = {
            "omnibus": omnibus,
            "project_name": "puppy" if puppy else "agent",
            "log_level": log_level,
            "overrides": overrides_cmd,
            "populate_s3_cache": "",
            "build_exclude": "coreos"
        }
        if omnibus_s3_cache:
            args['populate_s3_cache'] = " --populate-s3-cache "
        if skip_sign:
            env['SKIP_SIGN_MAC'] = 'true'
        env['PACKAGE_VERSION'] = get_version(ctx, include_git=True, url_safe=True)
        ctx.run(cmd.format(**args), env=env)


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


@task
def version(ctx, url_safe=False, git_sha_length=8):
    """
    Get the agent version.
    url_safe: get the version that is able to be addressed as a url
    git_sha_length: different versions of git have a different short sha length,
                    use this to explicitly set the version
                    (the windows builder and the default ubuntu version have such an incompatibility)
    """
    print(get_version(ctx, include_git=True, url_safe=url_safe, git_sha_length=git_sha_length))
