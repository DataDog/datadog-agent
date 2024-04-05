"""
Agent namespaced tasks
"""

import ast
import glob
import json
import os
import platform
import re
import shutil
import sys
import tempfile
from datetime import datetime

import requests
from invoke import task
from invoke.exceptions import Exit, ParseError

from tasks.build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from tasks.flavor import AgentFlavor
from tasks.go import deps
from tasks.libs.common.utils import (
    REPO_PATH,
    bin_name,
    cache_version,
    get_build_flags,
    get_embedded_path,
    get_goenv,
    get_version,
    has_both_python,
    load_release_versions,
    timed,
)
from tasks.rtloader import clean as rtloader_clean
from tasks.rtloader import install as rtloader_install
from tasks.rtloader import make as rtloader_make
from tasks.ssm import get_pfx_pass, get_signing_cert
from tasks.windows_resources import build_messagetable, build_rc, versioninfo_vars

# constants
BIN_DIR = os.path.join(".", "bin")
BIN_PATH = os.path.join(BIN_DIR, "agent")
AGENT_TAG = "datadog/agent:master"

BUNDLED_AGENTS = {
    # system-probe requires a working compilation environment for eBPF so we do not
    # enable it by default but we enable it in the released artifacts.
    AgentFlavor.base: ["process-agent", "trace-agent", "security-agent"],
}

AGENT_CORECHECKS = [
    "container",
    "containerd",
    "container_image",
    "container_lifecycle",
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
    "oracle",
    "oracle-dbm",
    "sbom",
    "systemd",
    "tcp_queue_length",
    "uptime",
    "winproc",
    "jetson",
    "telemetry",
    "orchestrator_pod",
    "orchestrator_ecs",
]

WINDOWS_CORECHECKS = [
    "agentcrashdetect",
    "windows_registry",
    "winkmem",
    "wincrashdetect",
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


@task(iterable=['bundle'])
def build(
    ctx,
    rebuild=False,
    race=False,
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
    development=True,
    skip_assets=False,
    install_path=None,
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
    cmake_options='',
    bundle=None,
    bundle_ebpf=False,
    agent_bin=None,
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
        rtloader_make(ctx, python_runtimes=python_runtimes, install_prefix=embedded_path, cmake_options=cmake_options)
        rtloader_install(ctx)

    ldflags, gcflags, env = get_build_flags(
        ctx,
        install_path=install_path,
        embedded_path=embedded_path,
        rtloader_root=rtloader_root,
        python_home_2=python_home_2,
        python_home_3=python_home_3,
        major_version=major_version,
        python_runtimes=python_runtimes,
    )

    bundled_agents = ["agent"]
    if sys.platform == 'win32':
        # Important for x-compiling
        env["CGO_ENABLED"] = "1"

        if arch == "x86":
            env["GOARCH"] = "386"

        build_messagetable(ctx, arch=arch)
        vars = versioninfo_vars(ctx, major_version=major_version, python_runtimes=python_runtimes, arch=arch)
        build_rc(
            ctx,
            "cmd/agent/windows_resources/agent.rc",
            arch=arch,
            vars=vars,
            out="cmd/agent/rsrc.syso",
        )
    else:
        bundled_agents += bundle or BUNDLED_AGENTS.get(flavor, [])

    if flavor.is_iot():
        # Iot mode overrides whatever passed through `--build-exclude` and `--build-include`
        build_tags = get_default_build_tags(build="agent", arch=arch, flavor=flavor)
    else:
        all_tags = set()
        if bundle_ebpf and "system-probe" in bundled_agents:
            all_tags.add("ebpf_bindata")

        for build in bundled_agents:
            all_tags.add("bundle_" + build.replace("-", "_"))
            include_tags = (
                get_default_build_tags(build=build, arch=arch, flavor=flavor)
                if build_include is None
                else filter_incompatible_tags(build_include.split(","), arch=arch)
            )

            exclude_tags = [] if build_exclude is None else build_exclude.split(",")
            build_tags = get_build_tags(include_tags, exclude_tags)

            all_tags |= set(build_tags)
        build_tags = list(all_tags)

    cmd = "go build -mod={go_mod} {race_opt} {build_type} -tags \"{go_build_tags}\" "

    if not agent_bin:
        agent_bin = os.path.join(BIN_PATH, bin_name("agent"))

    cmd += "-o {agent_bin} -gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/{flavor}"
    args = {
        "go_mod": go_mod,
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else "",
        "go_build_tags": " ".join(build_tags),
        "agent_bin": agent_bin,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
        "flavor": "iot-agent" if flavor.is_iot() else "agent",
    }
    ctx.run(cmd.format(**args), env=env)

    if embedded_path is None:
        embedded_path = get_embedded_path(ctx)

    for build in bundled_agents:
        if build == "agent":
            continue

        bundled_agent_dir = os.path.join(BIN_DIR, build)
        bundled_agent_bin = os.path.join(bundled_agent_dir, bin_name(build))
        agent_fullpath = os.path.normpath(os.path.join(embedded_path, "..", "bin", "agent", bin_name("agent")))

        if not os.path.exists(os.path.dirname(bundled_agent_bin)):
            os.mkdir(os.path.dirname(bundled_agent_bin))

        create_launcher(ctx, build, agent_fullpath, bundled_agent_bin)

    render_config(
        ctx,
        env=env,
        flavor=flavor,
        python_runtimes=python_runtimes,
        skip_assets=skip_assets,
        build_tags=build_tags,
        development=development,
        windows_sysprobe=windows_sysprobe,
    )


def create_launcher(ctx, agent, src, dst):
    cc = get_goenv(ctx, "CC")
    if not cc:
        print("Failed to find C compiler")
        raise Exit(code=1)

    cmd = "{cc} -DDD_AGENT_PATH='\"{agent_bin}\"' -DDD_AGENT='\"{agent}\"' -o {launcher_bin} ./cmd/agent/launcher/launcher.c"
    args = {
        "cc": cc,
        "agent": agent,
        "agent_bin": src,
        "launcher_bin": dst,
    }
    ctx.run(cmd.format(**args))


def render_config(ctx, env, flavor, python_runtimes, skip_assets, build_tags, development, windows_sysprobe):
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

    generate_config(ctx, build_type="security-agent", output_file="./cmd/agent/dist/security-agent.yaml", env=env)

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
        shutil.copytree("./cmd/agent/dist/checks/", os.path.join(dist_folder, "checks"), dirs_exist_ok=True)
        shutil.copytree("./cmd/agent/dist/utils/", os.path.join(dist_folder, "utils"), dirs_exist_ok=True)
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

    shutil.copy("./cmd/agent/dist/security-agent.yaml", os.path.join(dist_folder, "security-agent.yaml"))

    for check in AGENT_CORECHECKS if not flavor.is_iot() else IOT_AGENT_CORECHECKS:
        check_dir = os.path.join(dist_folder, f"conf.d/{check}.d/")
        shutil.copytree(f"./cmd/agent/dist/conf.d/{check}.d/", check_dir, dirs_exist_ok=True)
        # Ensure the config folders are not world writable
        os.chmod(check_dir, mode=0o755)

    ## add additional windows-only corechecks, only on windows. Otherwise the check loader
    ## on linux will throw an error because the module is not found, but the config is.
    if sys.platform == 'win32':
        for check in WINDOWS_CORECHECKS:
            check_dir = os.path.join(dist_folder, f"conf.d/{check}.d/")
            shutil.copytree(f"./cmd/agent/dist/conf.d/{check}.d/", check_dir, dirs_exist_ok=True)

    if "apm" in build_tags:
        shutil.copy("./cmd/agent/dist/conf.d/apm.yaml.default", os.path.join(dist_folder, "conf.d/apm.yaml.default"))
    if "process" in build_tags:
        shutil.copy(
            "./cmd/agent/dist/conf.d/process_agent.yaml.default",
            os.path.join(dist_folder, "conf.d/process_agent.yaml.default"),
        )

    shutil.copytree("./comp/core/gui/guiimpl/views", os.path.join(dist_folder, "views"), dirs_exist_ok=True)
    if development:
        shutil.copytree("./dev/dist/", dist_folder, dirs_exist_ok=True)


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
def exec(
    ctx,
    subcommand,
    config_path=None,
):
    """
    Execute 'agent <subcommand>' against the currently running Agent.

    This works against an agent run via `inv agent.run`.
    Basically this just simplifies creating the path for both the agent binary and config.
    """
    agent_bin = os.path.join(BIN_PATH, bin_name("agent"))
    config_path = os.path.join(BIN_PATH, "dist", "datadog.yaml") if not config_path else config_path
    ctx.run(f"{agent_bin} -c {config_path} {subcommand}")


@task
def system_tests(_):
    """
    Run the system testsuite.
    """
    pass


@task
def image_build(ctx, arch='amd64', base_dir="omnibus", python_version="2", skip_tests=False, tag=None, push=False):
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
    dockerfile_path = f"{build_context}/Dockerfile"
    list_of_files = glob.glob(os.path.join(pkg_dir, deb_glob))
    # get the last debian package built
    if not list_of_files:
        print(f"No debian package build found in {pkg_dir}")
        print("See agent.omnibus-build")
        raise Exit(code=1)
    latest_file = max(list_of_files, key=os.path.getctime)
    shutil.copy2(latest_file, build_context)

    if tag is None:
        tag = AGENT_TAG

    common_build_opts = f"-t {tag} -f {dockerfile_path}"
    if python_version not in BOTH_VERSIONS:
        common_build_opts = f"{common_build_opts} --build-arg PYTHON_VERSION={python_version}"

    # Build with the testing target
    if not skip_tests:
        ctx.run(f"docker build {common_build_opts} --platform linux/{arch} --target testing {build_context}")

    # Build with the release target
    ctx.run(f"docker build {common_build_opts} --platform linux/{arch} --target release {build_context}")
    if push:
        ctx.run(f"docker push {tag}")

    ctx.run(f"rm {build_context}/{deb_glob}")


@task
def hacky_dev_image_build(
    ctx,
    base_image=None,
    target_image="agent",
    target_tag="latest",
    push=False,
    signed_pull=False,
):
    if base_image is None:
        import requests
        import semver

        # Try to guess what is the latest release of the agent
        latest_release = semver.VersionInfo(0)
        tags = requests.get("https://gcr.io/v2/datadoghq/agent/tags/list")
        for tag in tags.json()['tags']:
            if not semver.VersionInfo.isvalid(tag):
                continue
            ver = semver.VersionInfo.parse(tag)
            if ver.prerelease or ver.build:
                continue
            if ver > latest_release:
                latest_release = ver
        base_image = f"gcr.io/datadoghq/agent:{latest_release}"

    # Extract the python library of the docker image
    with tempfile.TemporaryDirectory() as extracted_python_dir:
        ctx.run(
            f"docker run --rm '{base_image}' bash -c 'tar --create /opt/datadog-agent/embedded/{{bin,lib,include}}/*python*' | tar --directory '{extracted_python_dir}' --extract"
        )

        os.environ["DELVE"] = "1"
        os.environ["LD_LIBRARY_PATH"] = (
            os.environ.get("LD_LIBRARY_PATH", "") + f":{extracted_python_dir}/opt/datadog-agent/embedded/lib"
        )
        build(
            ctx,
            cmake_options=f'-DPython3_ROOT_DIR={extracted_python_dir}/opt/datadog-agent/embedded -DPython3_FIND_STRATEGY=LOCATION',
        )
        ctx.run(
            f'perl -0777 -pe \'s|{extracted_python_dir}(/opt/datadog-agent/embedded/lib/python\\d+\\.\\d+/../..)|substr $1."\\0"x length$&,0,length$&|e or die "pattern not found"\' -i dev/lib/libdatadog-agent-three.so'
        )

    with tempfile.NamedTemporaryFile(mode='w') as dockerfile:
        dockerfile.write(
            f'''FROM ubuntu:latest AS src

COPY . /usr/src/datadog-agent

RUN find /usr/src/datadog-agent -type f \\! -name \\*.go -print0 | xargs -0 rm
RUN find /usr/src/datadog-agent -type d -empty -print0 | xargs -0 rmdir

FROM ubuntu:latest AS bin

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get install -y patchelf

COPY bin/agent/agent                            /opt/datadog-agent/bin/agent/agent
COPY dev/lib/libdatadog-agent-rtloader.so.0.1.0 /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0
COPY dev/lib/libdatadog-agent-three.so          /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so

RUN patchelf --set-rpath /opt/datadog-agent/embedded/lib /opt/datadog-agent/bin/agent/agent
RUN patchelf --set-rpath /opt/datadog-agent/embedded/lib /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0
RUN patchelf --set-rpath /opt/datadog-agent/embedded/lib /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so

FROM golang:latest AS dlv

RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM {base_image}

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get install -y bash-completion less vim tshark && \
    apt-get clean

ENV DELVE_PAGER=less

COPY --from=dlv /go/bin/dlv /usr/local/bin/dlv
COPY --from=src /usr/src/datadog-agent {os.getcwd()}
COPY --from=bin /opt/datadog-agent/bin/agent/agent                                 /opt/datadog-agent/bin/agent/agent
COPY --from=bin /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0 /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0
COPY --from=bin /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so          /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so
RUN agent          completion bash > /usr/share/bash-completion/completions/agent
RUN process-agent  completion bash > /usr/share/bash-completion/completions/process-agent
RUN security-agent completion bash > /usr/share/bash-completion/completions/security-agent
RUN system-probe   completion bash > /usr/share/bash-completion/completions/system-probe
RUN trace-agent    completion bash > /usr/share/bash-completion/completions/trace-agent

ENV DD_SSLKEYLOGFILE=/tmp/sslkeylog.txt
'''
        )
        dockerfile.flush()

        target_image_name = f'{target_image}:{target_tag}'
        pull_env = {}
        if signed_pull:
            pull_env['DOCKER_CONTENT_TRUST'] = '1'
        ctx.run(f'docker build -t {target_image_name} -f {dockerfile.name} .', env=pull_env)

        if push:
            ctx.run(f'docker push {target_image_name}')


@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False, go_mod="mod", arch="x64"):
    """
    Run integration tests for the Agent
    """
    if install_deps:
        deps(ctx)

    if sys.platform == 'win32':
        return _windows_integration_tests(ctx, race=race, go_mod=go_mod, arch=arch)
    else:
        # TODO: See if these will function on Windows
        return _linux_integration_tests(ctx, race=race, remote_docker=remote_docker, go_mod=go_mod, arch=arch)


def _windows_integration_tests(ctx, race=False, go_mod="mod", arch="x64"):
    test_args = {
        "go_mod": go_mod,
        "go_build_tags": " ".join(get_default_build_tags(build="test", arch=arch)),
        "race_opt": "-race" if race else "",
        "exec_opts": "",
    }

    go_cmd = 'go test -mod={go_mod} {race_opt} -tags "{go_build_tags}" {exec_opts}'.format(**test_args)  # noqa: FS002

    tests = [
        {
            # Run eventlog tests with the Windows API, which depend on the EventLog service
            "dir": "./pkg/util/winutil/",
            'prefix': './eventlog/...',
            'extra_args': '-evtapi Windows',
        },
        {
            # Run eventlog tailer tests with the Windows API, which depend on the EventLog service
            "dir": ".",
            'prefix': './pkg/logs/tailers/windowsevent/...',
            'extra_args': '-evtapi Windows',
        },
        {
            # Run eventlog check tests with the Windows API, which depend on the EventLog service
            "dir": ".",
            'prefix': './comp/checks/windowseventlog/windowseventlogimpl/check/...',
            'extra_args': '-evtapi Windows',
        },
    ]

    for test in tests:
        with ctx.cd(f"{test['dir']}"):
            ctx.run(f"{go_cmd} {test['prefix']} {test['extra_args']}")


def _linux_integration_tests(ctx, race=False, remote_docker=False, go_mod="mod", arch="x64"):
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

    return env


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


def should_retry_bundle_install(res):
    # We sometimes get a Net::HTTPNotFound error when fetching the
    # license-scout gem. This is a transient error, so we retry the bundle install
    if "Net::HTTPNotFound:" in res.stderr:
        return True
    return False


def _send_build_metrics(ctx, overall_duration):
    # We only want to generate those metrics from the CI
    if sys.platform == 'win32':
        src_dir = "C:/buildroot/datadog-agent"
        aws_cmd = "aws.cmd"
    else:
        src_dir = os.environ.get('CI_PROJECT_DIR')
        aws_cmd = "aws"
    job_name = os.environ.get('CI_JOB_NAME_SLUG')
    branch = os.environ.get('CI_COMMIT_REF_NAME')
    pipeline_id = os.environ.get('CI_PIPELINE_ID')
    if not job_name or not branch or not src_dir or not pipeline_id:
        print(
            '''Missing required environment variables, this is probably not a CI job.
                  skipping sending build metrics'''
        )
        return

    series = []
    timestamp = int(datetime.now().timestamp())
    with open(f'{src_dir}/omnibus/pkg/build-summary.json') as summary_json:
        j = json.load(summary_json)
        # Various software build durations are all sent as the `datadog.agent.build.duration` metric
        # with a specific tag for each software.
        for software, metrics in j['build'].items():
            series.append(
                {
                    'metric': 'datadog.agent.build.duration',
                    'points': [{'timestamp': timestamp, 'value': metrics['build_duration']}],
                    'tags': [
                        f'software:{software}',
                        f'cached:{metrics["cached"]}',
                        f'job:{job_name}',
                        f'branch:{branch}',
                        f'pipeline:{pipeline_id}',
                    ],
                    'unit': 'seconds',
                    'type': 0,
                }
            )
        # We also provide the total duration for the omnibus build as a separate metric
        series.append(
            {
                'metric': 'datadog.agent.build.total',
                'points': [{'timestamp': timestamp, 'value': overall_duration}],
                'tags': [
                    f'job:{job_name}',
                    f'branch:{branch}',
                    f'pipeline:{pipeline_id}',
                ],
                'unit': 'seconds',
                'type': 0,
            }
        )
        # Stripping might not always be enabled so we conditionally read the metric
        if "strip" in j:
            series.append(
                {
                    'metric': 'datadog.agent.build.strip',
                    'points': [{'timestamp': timestamp, 'value': j['strip']}],
                    'tags': [
                        f'job:{job_name}',
                        f'branch:{branch}',
                        f'pipeline:{pipeline_id}',
                    ],
                    'unit': 'seconds',
                    'type': 0,
                }
            )
        # And all packagers duration as another separated metric
        for packager, duration in j['packaging'].items():
            series.append(
                {
                    'metric': 'datadog.agent.package.duration',
                    'points': [{'timestamp': timestamp, 'value': duration}],
                    'tags': [
                        f'job:{job_name}',
                        f'branch:{branch}',
                        f'packager:{packager}',
                        f'pipeline:{pipeline_id}',
                    ],
                    'unit': 'seconds',
                    'type': 0,
                }
            )
    dd_api_key = ctx.run(
        f'{aws_cmd} ssm get-parameter --region us-east-1 --name {os.environ["API_KEY_ORG2_SSM_NAME"]} --with-decryption --query "Parameter.Value" --out text',
        hide=True,
    ).stdout.strip()
    headers = {'Accept': 'application/json', 'Content-Type': 'application/json', 'DD-API-KEY': dd_api_key}
    r = requests.post("https://api.datadoghq.com/api/v2/series", json={'series': series}, headers=headers)
    if r.ok:
        print('Successfully sent build metrics to DataDog')
    else:
        print(f'Failed to send build metrics to DataDog: {r.status_code}')
        print(r.text)


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
    go_mod_cache=None,
    python_mirror=None,
    pip_config_file="pip.conf",
    host_distribution=None,
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

    target_project = "agent"
    if flavor.is_iot():
        target_project = "iot-agent"
    elif agent_binaries:
        target_project = "agent-binaries"

    # Get the python_mirror from the PIP_INDEX_URL environment variable if it is not passed in the args
    python_mirror = python_mirror or os.environ.get("PIP_INDEX_URL")

    # If a python_mirror is set then use it for pip by adding it in the pip.conf file
    pip_index_url = f"[global]\nindex-url = {python_mirror}" if python_mirror else ""

    # We're passing the --index-url arg through a pip.conf file so that omnibus doesn't leak the token
    with open(pip_config_file, 'w') as f:
        f.write(pip_index_url)

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
            host_distribution=host_distribution,
        )

    # Delete the temporary pip.conf file once the build is done
    os.remove(pip_config_file)

    print("Build component timing:")
    if not skip_deps:
        print(f"Deps:    {deps_elapsed.duration}")
    print(f"Bundle:  {bundle_elapsed.duration}")
    print(f"Omnibus: {omnibus_elapsed.duration}")
    _send_build_metrics(ctx, omnibus_elapsed.duration)


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
def version(
    ctx,
    url_safe=False,
    omnibus_format=False,
    git_sha_length=7,
    major_version='7',
    version_cached=False,
    pipeline_id=None,
    include_git=True,
    include_pre=True,
):
    """
    Get the agent version.
    url_safe: get the version that is able to be addressed as a url
    omnibus_format: performs the same transformations omnibus does on version names to
                    get the exact same string that's used in package names
    git_sha_length: different versions of git have a different short sha length,
                    use this to explicitly set the version
                    (the windows builder and the default ubuntu version have such an incompatibility)
    version_cached: save the version inside a "agent-version.cache" that will be reused
                    by each next call of version.
    """
    if version_cached:
        cache_version(ctx, git_sha_length=git_sha_length)

    version = get_version(
        ctx,
        include_git=include_git,
        url_safe=url_safe,
        git_sha_length=git_sha_length,
        major_version=major_version,
        include_pipeline_id=True,
        pipeline_id=pipeline_id,
        include_pre=include_pre,
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


@task()
def generate_config(ctx, build_type, output_file, env=None):
    """
    Generates the datadog.yaml configuration file.
    """
    args = {
        "go_file": "./pkg/config/render_config.go",
        "build_type": build_type,
        "template_file": "./pkg/config/config_template.yaml",
        "output_file": output_file,
    }
    cmd = "go run {go_file} {build_type} {template_file} {output_file}"
    return ctx.run(cmd.format(**args), env=env or {})
