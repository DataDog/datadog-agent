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

from .utils import (
    bin_name,
    get_build_flags,
    get_version_numeric_only,
    load_release_versions,
    get_version,
    has_both_python,
    get_win_py_runtime_var,
)
from .utils import REPO_PATH
from .utils import do_go_rename, do_sed_rename
from .build_tags import get_build_tags, get_default_build_tags, LINUX_ONLY_TAGS, WINDOWS_32BIT_EXCLUDE_TAGS
from .go import deps, generate
from .docker import pull_base_images
from .ssm import get_signing_cert, get_pfx_pass
from .rtloader import make as rtloader_make
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
    "load",
    "memory",
    "network",
    "ntp",
    "uptime",
    "systemd",
]


@task
def apply_branding(ctx):
    """
    Apply stackstate branding
    """
    sts_camel_replace = 's/Data[dD]og/StackState/g'
    sts_lower_replace = 's/datadog/stackstate/g'
    datadog_metrics_replace = 's/"datadog./"stackstate./g'
    datadog_checks_replace = 's/"datadog_checks./"stackstate_checks./g'

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
    do_go_rename(ctx, '"\\"/opt/datadog-agent/run\\" -> \\"/opt/stackstate-agent/run\\""', "./pkg/config")

    # cmd/agent/common/common_windows.go
    do_sed_rename(ctx, 's/"programdata\\\\\\\\datadog"/"programdata\\\\\\\\stackstate"/g',
                  "./cmd/agent/common/common_windows.go")
    do_sed_rename(ctx, 's/"ProgramData\\\\\\\\datadog"/"ProgramData\\\\\\\\StackState"/g',
                  "./cmd/agent/common/common_windows.go")
    do_sed_rename(ctx, 's/"Datadog"/"Stackstate"/g',
                  "./cmd/agent/common/common_windows.go")
    do_sed_rename(ctx, 's/"ProgramData\\\\\\\\DataDog"/"ProgramData\\\\\\\\StackState"/g',
                  "./cmd/agent/common/common_windows.go")
    do_sed_rename(ctx, 's/"SOFTWARE\\\\DataDog\\\\"/"SOFTWARE\\\\\StackState\\\\"/g',
                  "./cmd/agent/common/common_windows.go")
    do_sed_rename(ctx, 's/"datadog.conf"/"stackstate.conf"/g',
                  "./cmd/agent/common/common_windows.go")
    # systray.go
    do_sed_rename(ctx, 's/"programdata\\\\\\\\datadog"/"programdata\\\\\\\\stackstate"/g',
                  "./cmd/systray/systray.go")
    do_sed_rename(ctx, 's/"ProgramData\\\\\\\\datadog"/"ProgramData\\\\\\\\StackState"/g',
                  "./cmd/systray/systray.go")
    do_sed_rename(ctx, 's/"Datadog"/"Stackstate"/g',
                  "./cmd/systray/systray.go")
    do_sed_rename(ctx, 's/"ProgramData\\\\\\\\DataDog"/"ProgramData\\\\\\\\StackState"/g',
                  "./cmd/systray/systray.go")
    # pkg/config/config_windows.go
    do_sed_rename(ctx, 's/"programdata\\\\\\\\datadog"/"programdata\\\\\\\\stackstate"/g',
                  "./pkg/config/config_windows.go")
    do_sed_rename(ctx, 's/"ProgramData\\\\\\\\datadog"/"ProgramData\\\\\\\\StackState"/g',
                  "./pkg/config/config_windows.go")
    do_sed_rename(ctx, 's/"Datadog"/"Stackstate"/g',
                  "./pkg/config/config_windows.go")
    do_sed_rename(ctx, 's/"ProgramData\\\\\\\\DataDog"/"ProgramData\\\\\\\\StackState"/g',
                  "./pkg/config/config_windows.go")
    do_sed_rename(ctx, 's/"SOFTWARE\\\\DataDog\\\\"/"SOFTWARE\\\\\StackState\\\\"/g',
                  "./pkg/config/config_windows.go")
    do_sed_rename(ctx, 's/"datadog.conf"/"stackstate.conf"/g',
                  "./pkg/config/config_windows.go")
    # pkg/pidfile/pidfile_windows.go
    do_sed_rename(ctx, 's/"programdata\\\\\\\\datadog"/"programdata\\\\\\\\stackstate"/g',
                  "./pkg/pidfile/pidfile_windows.go")
    do_sed_rename(ctx, 's/"ProgramData\\\\\\\\datadog"/"ProgramData\\\\\\\\StackState"/g',
                  "./pkg/pidfile/pidfile_windows.go")
    do_sed_rename(ctx, 's/"ProgramData\\\\\\\\DataDog"/"ProgramData\\\\\\\\StackState"/g',
                  "./pkg/pidfile/pidfile_windows.go")
    do_sed_rename(ctx, 's/"Datadog"/"Stackstate"/g',
                  "./pkg/pidfile/pidfile_windows.go")
    do_sed_rename(ctx, 's/"datadog"/"stackstate"/g',
                  "./pkg/pidfile/pidfile_windows.go")
    # pkg/trace/config/config_windows.go
    do_sed_rename(ctx, 's/"programdata\\\\\\\\datadog"/"programdata\\\\\\\\stackstate"/g',
                  "./pkg/trace/config/config_windows.go")
    do_sed_rename(ctx, 's/"ProgramData\\\\\\\\datadog"/"ProgramData\\\\\\\\StackState"/g',
                  "./pkg/trace/config/config_windows.go")
    do_sed_rename(ctx, 's/"Datadog"/"Stackstate"/g',
                  "./pkg/trace/config/config_windows.go")
    do_sed_rename(ctx, 's/"ProgramData\\\\\\\\DataDog"/"ProgramData\\\\\\\\StackState"/g',
                  "./pkg/trace/config/config_windows.go")
    do_sed_rename(ctx, 's/"datadog.conf"/"stackstate.conf"/g',
                  "./pkg/trace/config/config_windows.go")
    # pkg/trace/flags/flags_windows.go
    do_sed_rename(ctx, 's/"programdata\\\\\\\\datadog"/"programdata\\\\\\\\stackstate"/g',
                  "./pkg/trace/flags/flags_windows.go")
    do_sed_rename(ctx, 's/"ProgramData\\\\\\\\datadog"/"ProgramData\\\\\\\\StackState"/g',
                  "./pkg/trace/flags/flags_windows.go")
    do_sed_rename(ctx, 's/"Datadog"/"Stackstate"/g',
                  "./pkg/trace/flags/flags_windows.go")
    do_sed_rename(ctx, 's/"ProgramData\\\\\\\\DataDog"/"ProgramData\\\\\\\\StackState"/g',
                  "./pkg/trace/flags/flags_windows.go")

    # Commands
    do_sed_rename(ctx, sts_lower_replace, "./cmd/agent/app/integrations.go")
    do_sed_rename(ctx, sts_lower_replace, "./cmd/agent/app/dependent_services_windows.go")
    do_sed_rename(ctx, sts_lower_replace, "./cmd/agent/app/launchgui.go")
    do_sed_rename(ctx, 's/Datadog Agent/StackState Agent/g', "./cmd/agent/app/launchgui.go")
    do_sed_rename(ctx, 's/Datadog Agent/StackState Agent/g', "./cmd/agent/app/start.go")
    do_sed_rename(ctx, sts_lower_replace, "./cmd/agent/app/app.go")
    do_sed_rename(ctx, sts_lower_replace, "./cmd/agent/app/integrations.go")
    do_sed_rename(ctx, 's/Datadog integration/StackState integration/g', "./cmd/agent/app/integrations.go")
    do_go_rename(ctx, '"\\"Collect a flare and send it to Datadog\\" -> \\"Collect a flare and send it to StackState\\""', "./cmd/agent/app")
    do_sed_rename(ctx, sts_lower_replace, "./cmd/agent/app/regimport_windows.go")

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

    # Trace agent
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

    # Trace Agent Metrics
    do_sed_rename(ctx, datadog_metrics_replace, "./pkg/trace/api/api.go")
    do_sed_rename(ctx, datadog_metrics_replace, "./pkg/trace/api/responses.go")
    do_sed_rename(ctx, datadog_metrics_replace, "./pkg/trace/agent/run.go")
    do_sed_rename(ctx, datadog_metrics_replace, "./pkg/trace/agent/agent.go")
    do_go_rename(ctx, '"\\"datadog.conf\\" -> \\"stackstate.conf\\""', "./pkg/trace/agent")
    do_sed_rename(ctx, datadog_metrics_replace, "./pkg/trace/event/sampler_max_eps.go")
    do_sed_rename(ctx, datadog_metrics_replace, "./pkg/trace/writer/trace.go")
    do_sed_rename(ctx, datadog_metrics_replace, "./pkg/trace/writer/stats.go")
    do_sed_rename(ctx, datadog_metrics_replace, "./pkg/trace/writer/stats_test.go")
    do_sed_rename(ctx, datadog_metrics_replace, "./pkg/trace/info/stats.go")

    # Defaults
    do_go_rename(ctx, '"\\"/etc/datadog-agent\\" -> \\"/etc/stackstate-agent\\""', "./cmd/agent/common")
    do_go_rename(ctx, '"\\"/var/log/datadog/agent.log\\" -> \\"/var/log/stackstate-agent/agent.log\\""', "./cmd/agent/common")
    do_go_rename(ctx, '"\\"/var/log/datadog/cluster-agent.log\\" -> \\"/var/log/stackstate-agent/cluster-agent.log\\""', "./cmd/agent/common")
    do_go_rename(ctx, '"\\"datadog.yaml\\" -> \\"stackstate.yaml\\""', "./cmd/agent")
    do_go_rename(ctx, '"\\"datadog.yaml\\" -> \\"stackstate.yaml\\""', "./pkg/config")
    do_go_rename(ctx, '"\\"datadog.conf\\" -> \\"stackstate.conf\\""', "./cmd/agent")
    do_go_rename(ctx, '"\\"path to directory containing datadog.yaml\\" -> \\"path to directory containing stackstate.yaml\\""', "./cmd")
    do_go_rename(ctx, '"\\"unable to load Datadog config file: %s\\" -> \\"unable to load StackState config file: %s\\""', "./cmd/agent/common")
    do_go_rename(ctx, '"\\"Starting Datadog Agent v%v\\" -> \\"Starting StackState Agent v%v\\""', "./cmd/agent/app")

    # Dist config templates
    do_sed_rename(ctx, sts_lower_replace, "./cmd/agent/dist/conf.d/go_expvar.d/agent_stats.yaml.example")
    do_sed_rename(ctx, sts_lower_replace, "./cmd/agent/dist/conf.d/apm.yaml.default")
    do_sed_rename(ctx, 's/dd/sts/g', "./cmd/agent/dist/dd-agent")
    do_sed_rename(ctx, sts_lower_replace, "./cmd/agent/dist/dd-agent")

    # Hardcoded checks and metrics
    do_sed_rename(ctx, sts_lower_replace, "./pkg/aggregator/aggregator.go")

    # Windows defaults
    do_sed_rename(ctx, sts_camel_replace, "./cmd/agent/agent.rc")
    do_sed_rename(ctx, sts_camel_replace, "./cmd/trace-agent/windows_resources/trace-agent.rc")
    do_sed_rename(ctx, sts_camel_replace, "./cmd/agent/app/install_service_windows.go")
    do_sed_rename(ctx, sts_lower_replace, "./cmd/agent/app/dependent_services_windows.go")
    # replace strings NOT containing certain pattern
    do_sed_rename(ctx, '/config/! s/Data[dD]og/StackState/g', "./cmd/agent/common/common_windows.go")
    do_sed_rename(ctx, sts_lower_replace, "./cmd/agent/common/common_windows.go")
    do_sed_rename(ctx, 's/dd_url/sts_url/', "./cmd/agent/common/common_windows.go")
    do_sed_rename(ctx, sts_lower_replace, "./cmd/dogstatsd/main_windows.go")
    do_sed_rename(ctx, sts_camel_replace, "./pkg/config/config_windows.go")

    # Windows MSI installation
    do_sed_rename(ctx, sts_camel_replace, "./omnibus/resources/agent/msi/localization-en-us.wxl.erb")
    do_sed_rename(ctx, 's/"datadog\.yaml\.example"/"stackstate\.yaml\.example"/', "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, 's/datadoghq\.com/www\.stackstate\.com/', "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, sts_camel_replace, "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, sts_lower_replace, "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, 's/DATADOG/STACKSTATE/', "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, 's/dd_url/sts_url/', "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, 's/\[.*DD_URL\]/\[STS_URL\]/', "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, sts_camel_replace, "./omnibus/resources/agent/msi/bundle.wxs.erb")
    do_sed_rename(ctx, 's/dd_logo_side\\.png/sts_logo_side\\.png/', "./omnibus/resources/agent/msi/bundle.wxs.erb")

    # Windows SysTray and GUI
    tray_replace = 's/ddtray/ststray/'
    do_sed_rename(ctx, sts_lower_replace, "./cmd/systray/doservicecontrol.go")
    do_sed_rename(ctx, sts_camel_replace, "./cmd/systray/systray.go")
    do_sed_rename(ctx, tray_replace, "./cmd/systray/systray.go")
    do_sed_rename(ctx, sts_camel_replace, "./cmd/systray/systray.rc")
    do_sed_rename(ctx, tray_replace, "./cmd/systray/systray.rc")
    do_sed_rename(ctx, tray_replace, "./omnibus/resources/agent/msi/source.wxs.erb")
    do_sed_rename(ctx, tray_replace, "./tasks/systray.py")
    do_sed_rename(ctx, sts_lower_replace, "./cmd/agent/gui/views/templates/index.tmpl")
    do_sed_rename(ctx, 's/"DataDog Agent 6"/"StackState Agent 2"/', "./cmd/agent/gui/views/templates/index.tmpl")
    do_sed_rename(ctx, sts_camel_replace, "./cmd/agent/gui/views/templates/index.tmpl")
    do_sed_rename(ctx, sts_camel_replace, "./cmd/agent/gui/views/private/js/javascript.js")

    # stackstate_checks
    do_go_rename(ctx, '"\\"datadog_checks\\" -> \\"stackstate_checks\\""', "./cmd/agent/app")
    do_sed_rename(ctx, 's/datadog_checks_base/stackstate_checks_base/g', "./cmd/agent/app/integrations.go")
    do_go_rename(ctx, '"\\"datadog_checks\\" -> \\"stackstate_checks\\""', "./pkg/collector/python")
    do_go_rename(ctx, '"\\"An error occurred while grabbing the python datadog integration list\\" -> \\"An error occurred while grabbing the python StackState integration list\\""', "./pkg/collector/python")
#    do_sed_rename(ctx, datadog_checks_replace, "./pkg/collector/python/loader.go")
    do_sed_rename(ctx, datadog_metrics_replace, "./pkg/collector/runner/runner.go")

    # cluster agent client
    do_go_rename(ctx, '"\\"datadog-cluster-agent\\" -> \\"stackstate-cluster-agent\\""', "./pkg/config")
    do_sed_rename(ctx, 's/Datadog Cluster Agent/StackState Cluster Agent/g', "./pkg/util/clusteragent/clusteragent.go")

    # kubernetes openmetrics annotations
    do_sed_rename(ctx, 's/ad.datadoghq.com/ad.stackstate.com/g', "./pkg/autodiscovery/listeners/kubelet.go")
    do_sed_rename(ctx, 's/ad.datadoghq.com/ad.stackstate.com/g', "./pkg/autodiscovery/listeners/kube_services.go")
    do_sed_rename(ctx, 's/ad.datadoghq.com/ad.stackstate.com/g', "./pkg/autodiscovery/providers/kubelet.go")
    do_sed_rename(ctx, 's/ad.datadoghq.com/ad.stackstate.com/g', "./pkg/autodiscovery/providers/kube_services.go")
    do_sed_rename(ctx, 's/ad.datadoghq.com/ad.stackstate.com/g', "./pkg/tagger/collectors/kubelet_extract.go")
    do_sed_rename(ctx, 's/ad.datadoghq.com/ad.stackstate.com/g', "./pkg/util/kubernetes/kubelet/kubelet.go")

    # docker/ecs openmetrics annotations
    do_sed_rename(ctx, 's/com.datadoghq.ad/com.stackstate.ad/g', "./pkg/autodiscovery/listeners/common.go")
    do_sed_rename(ctx, 's/com.datadoghq.ad/com.stackstate.ad/g', "./pkg/autodiscovery/providers/docker.go")
    do_sed_rename(ctx, 's/com.datadoghq.ad/com.stackstate.ad/g', "./pkg/autodiscovery/providers/ecs.go")

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
    major_version='',
    python_runtimes='3',
    arch='x64',
    exclude_rtloader=False,
    go_mod="vendor",
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
    build_include = DEFAULT_BUILD_TAGS if build_include is None else build_include.split(",")
    build_exclude = [] if build_exclude is None else build_exclude.split(",")

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

    if not sys.platform.startswith('linux'):
        for ex in LINUX_ONLY_TAGS:
            if ex not in build_exclude:
                build_exclude.append(ex)

    if sys.platform == 'win32' and arch == "x86":
        for ex in WINDOWS_32BIT_EXCLUDE_TAGS:
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
            build_arch=arch,
        )
        command += "-i cmd/agent/agent.rc -O coff -o cmd/agent/rsrc.syso"
        ctx.run(command, env=env)

    if iot:
        # Iot mode overrides whatever passed through `--build-exclude` and `--build-include`
        build_tags = get_default_build_tags(iot=True)
    else:
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
    print ("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~")
    print(cmd.format(**args))
    print("~~~~~~")
    print("~~~")
    print(ldflags)
    print("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~")
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
    if sys.platform != 'win32':
        cmd = "go run ./pkg/config/render_config.go system-probe ./pkg/config/config_template.yaml ./cmd/agent/dist/system-probe.yaml"
        ctx.run(cmd, env=env)

    if not skip_assets:
        refresh_assets(ctx, build_tags, development=development, iot=iot)


@task
def refresh_assets(ctx, build_tags, development=True, iot=False):
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
        bin_ddagent = os.path.join(BIN_PATH, "sts-agent")
        shutil.move(os.path.join(dist_folder, "dd-agent"), bin_ddagent)

    # System probe not supported on windows
    if sys.platform.startswith('linux'):
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
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False, go_mod="vendor"):
    """
    Run integration tests for the Agent
    """
    if install_deps:
        deps(ctx)

    test_args = {
        "go_mod": go_mod,
        "go_build_tags": " ".join(get_default_build_tags()),
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
    major_version='',
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
