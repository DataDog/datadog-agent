"""
Agent namespaced tasks
"""

import ast
import glob
import os
import platform
import re
import shutil
import sys
import tempfile

from invoke import task
from invoke.exceptions import Exit, ParseError

from tasks.build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from tasks.devcontainer import run_on_devcontainer
from tasks.flavor import AgentFlavor
from tasks.libs.common.utils import (
    REPO_PATH,
    bin_name,
    get_build_flags,
    get_embedded_path,
    get_goenv,
    get_version,
    gitlab_section,
)
from tasks.libs.releasing.version import create_version_json
from tasks.rtloader import clean as rtloader_clean
from tasks.rtloader import install as rtloader_install
from tasks.rtloader import make as rtloader_make
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

if sys.platform == "win32":
    # Our `ridk enable` toolchain puts Ruby's bin dir at the front of the PATH
    # This dir contains `aws.rb` which will execute if we just call `aws`,
    # so we need to be explicit about the executable extension/path
    # NOTE: awscli seems to have a bug where running "aws.cmd", quoted, without a full path,
    #       causes it to fail due to not searching the PATH.
    # NOTE: The full path to `aws.cmd` is likely to contain spaces, so if the full path is
    #       used instead, it must be quoted when passed to ctx.run.
    # This unfortunately means that the quoting requirements are different if you use
    # the full path or just the filename.
    # aws.cmd -> awscli v1 from Python env
    AWS_CMD = "aws.cmd"
    # TODO: can we use use `aws.exe` from AWSCLIv2? E2E expects v2.
else:
    AWS_CMD = "aws"

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
    "cisco_sdwan",
    "network_path",
    "service_discovery",
]

WINDOWS_CORECHECKS = [
    "agentcrashdetect",
    "sbom",
    "windows_registry",
    "winkmem",
    "wincrashdetect",
    "win32_event_log",
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
@run_on_devcontainer
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
    exclude_rtloader=False,
    include_sds=False,
    go_mod="mod",
    windows_sysprobe=False,
    cmake_options='',
    bundle=None,
    bundle_ebpf=False,
    agent_bin=None,
    run_on=None,  # noqa: U100, F841. Used by the run_on_devcontainer decorator
):
    """
    Build the agent. If the bits to include in the build are not specified,
    the values from `invoke.yaml` will be used.

    Example invokation:
        inv agent.build --build-exclude=systemd
    """
    flavor = AgentFlavor[flavor]

    if flavor.is_ot():
        # for agent build purposes the UA agent is just like base
        flavor = AgentFlavor.base

    if not exclude_rtloader and not flavor.is_iot():
        # If embedded_path is set, we should give it to rtloader as it should install the headers/libs
        # in the embedded path folder because that's what is used in get_build_flags()
        with gitlab_section("Install embedded rtloader", collapsed=True):
            rtloader_make(ctx, install_prefix=embedded_path, cmake_options=cmake_options)
            rtloader_install(ctx)

    ldflags, gcflags, env = get_build_flags(
        ctx,
        install_path=install_path,
        embedded_path=embedded_path,
        rtloader_root=rtloader_root,
        python_home_2=python_home_2,
        python_home_3=python_home_3,
        major_version=major_version,
    )

    bundled_agents = ["agent"]
    if sys.platform == 'win32':
        # Important for x-compiling
        env["CGO_ENABLED"] = "1"

        build_messagetable(ctx)
        vars = versioninfo_vars(ctx, major_version=major_version)
        build_rc(
            ctx,
            "cmd/agent/windows_resources/agent.rc",
            vars=vars,
            out="cmd/agent/rsrc.syso",
        )
    else:
        bundled_agents += bundle or BUNDLED_AGENTS.get(flavor, [])

    if flavor.is_iot():
        # Iot mode overrides whatever passed through `--build-exclude` and `--build-include`
        build_tags = get_default_build_tags(build="agent", flavor=flavor)
    else:
        all_tags = set()
        if bundle_ebpf and "system-probe" in bundled_agents:
            all_tags.add("ebpf_bindata")

        for build in bundled_agents:
            all_tags.add("bundle_" + build.replace("-", "_"))
            include_tags = (
                get_default_build_tags(build=build, flavor=flavor)
                if build_include is None
                else filter_incompatible_tags(build_include.split(","))
            )

            exclude_tags = [] if build_exclude is None else build_exclude.split(",")
            build_tags = get_build_tags(include_tags, exclude_tags)

            all_tags |= set(build_tags)
        build_tags = list(all_tags)

    cmd = "go build -mod={go_mod} {race_opt} {build_type} -tags \"{go_build_tags}\" "

    if not agent_bin:
        agent_bin = os.path.join(BIN_PATH, bin_name("agent"))

    if include_sds:
        build_tags.append("sds")

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
    with gitlab_section("Build agent", collapsed=True):
        ctx.run(cmd.format(**args), env=env)

    if embedded_path is None:
        embedded_path = get_embedded_path(ctx)
        assert embedded_path, "Failed to find embedded path"

    for build in bundled_agents:
        if build == "agent":
            continue

        bundled_agent_dir = os.path.join(BIN_DIR, build)
        bundled_agent_bin = os.path.join(bundled_agent_dir, bin_name(build))
        agent_fullpath = os.path.normpath(os.path.join(embedded_path, "..", "bin", "agent", bin_name("agent")))

        if not os.path.exists(os.path.dirname(bundled_agent_bin)):
            os.mkdir(os.path.dirname(bundled_agent_bin))

        create_launcher(ctx, build, agent_fullpath, bundled_agent_bin)

    with gitlab_section("Generate configuration files", collapsed=True):
        render_config(
            ctx,
            env=env,
            flavor=flavor,
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


def render_config(ctx, env, flavor, skip_assets, build_tags, development, windows_sysprobe):
    # Remove cross-compiling bits to render config
    env.update({"GOOS": "", "GOARCH": ""})

    # Render the Agent configuration file template
    build_type = "agent-py3"
    if flavor.is_iot():
        build_type = "iot-agent"

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

    # add additional windows-only corechecks, only on windows. Otherwise the check loader
    # on linux will throw an error because the module is not found, but the config is.
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
    base_dir = base_dir or os.environ["OMNIBUS_BASE_DIR"]
    pkg_dir = os.path.join(base_dir, 'pkg')
    deb_glob = f'datadog-agent*_{arch}.deb'
    dockerfile_path = f"{build_context}/Dockerfile"
    list_of_files = glob.glob(os.path.join(pkg_dir, deb_glob))
    # get the last debian package built
    if not list_of_files:
        print(f"No debian package build found in {pkg_dir}")
        print("See omnibus.build")
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
    process_agent=False,
    trace_agent=False,
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

    copy_extra_agents = ""
    if process_agent:
        from tasks.process_agent import build as process_agent_build

        process_agent_build(ctx, bundle=False)
        copy_extra_agents += "COPY bin/process-agent/process-agent /opt/datadog-agent/embedded/bin/process-agent\n"
    if trace_agent:
        from tasks.trace_agent import build as trace_agent_build

        trace_agent_build(ctx)
        copy_extra_agents += "COPY bin/trace-agent/trace-agent /opt/datadog-agent/embedded/bin/trace-agent\n"

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

FROM {base_image} AS bash_completion

RUN apt-get update && \
    apt-get install -y gawk

RUN awk -i inplace '!/^#/ {{uncomment=0}} uncomment {{gsub(/^#/, "")}} /# enable bash completion/ {{uncomment=1}} {{print}}' /etc/bash.bashrc

FROM {base_image}

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get install -y bash-completion less vim tshark && \
    apt-get clean

ENV DELVE_PAGER=less

COPY --from=dlv /go/bin/dlv /usr/local/bin/dlv
COPY --from=bash_completion /etc/bash.bashrc /etc/bash.bashrc
COPY --from=src /usr/src/datadog-agent {os.getcwd()}
COPY --from=bin /opt/datadog-agent/bin/agent/agent                                 /opt/datadog-agent/bin/agent/agent
COPY --from=bin /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0 /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0
COPY --from=bin /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so          /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so
{copy_extra_agents}
RUN agent          completion bash > /usr/share/bash-completion/completions/agent
RUN process-agent  completion bash > /usr/share/bash-completion/completions/process-agent
RUN security-agent completion bash > /usr/share/bash-completion/completions/security-agent
RUN system-probe   completion bash > /usr/share/bash-completion/completions/system-probe
RUN trace-agent    completion bash > /usr/share/bash-completion/completions/trace-agent

ENV DD_SSLKEYLOGFILE=/tmp/sslkeylog.txt
'''
        )
        dockerfile.flush()

        pull_env = {}
        if signed_pull:
            pull_env['DOCKER_CONTENT_TRUST'] = '1'
        ctx.run(f'docker build -t {target_image} -f {dockerfile.name} .', env=pull_env)

        if push:
            ctx.run(f'docker push {target_image}')


@task
def integration_tests(ctx, race=False, remote_docker=False, go_mod="mod", timeout=""):
    """
    Run integration tests for the Agent
    """

    if sys.platform == 'win32':
        return _windows_integration_tests(ctx, race=race, go_mod=go_mod, timeout=timeout)
    else:
        # TODO: See if these will function on Windows
        return _linux_integration_tests(ctx, race=race, remote_docker=remote_docker, go_mod=go_mod, timeout=timeout)


def _windows_integration_tests(ctx, race=False, go_mod="mod", timeout=""):
    test_args = {
        "go_mod": go_mod,
        "go_build_tags": " ".join(get_default_build_tags(build="test")),
        "race_opt": "-race" if race else "",
        "exec_opts": "",
        "timeout_opt": f"-timeout {timeout}" if timeout else "",
    }

    go_cmd = 'go test {timeout_opt} -mod={go_mod} {race_opt} -tags "{go_build_tags}" {exec_opts}'.format(**test_args)  # noqa: FS002

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
            # Don't include submodules, since the `-evtapi` flag is not defined in them
            'prefix': './comp/checks/windowseventlog/windowseventlogimpl/check',
            'extra_args': '-evtapi Windows',
        },
    ]

    for test in tests:
        with ctx.cd(f"{test['dir']}"):
            ctx.run(f"{go_cmd} {test['prefix']} {test['extra_args']}")


def _linux_integration_tests(ctx, race=False, remote_docker=False, go_mod="mod", timeout=""):
    test_args = {
        "go_mod": go_mod,
        "go_build_tags": " ".join(get_default_build_tags(build="test")),
        "race_opt": "-race" if race else "",
        "exec_opts": "",
        "timeout_opt": f"-timeout {timeout}" if timeout else "",
    }

    # since Go 1.13, the -exec flag of go test could add some parameters such as -test.timeout
    # to the call, we don't want them because while calling invoke below, invoke
    # thinks that the parameters are for it to interpret.
    # we're calling an intermediate script which only pass the binary name to the invoke task.
    if remote_docker:
        test_args["exec_opts"] = f"-exec \"{os.getcwd()}/test/integration/dockerize_tests.sh\""

    go_cmd = 'go test {timeout_opt} -mod={go_mod} {race_opt} -tags "{go_build_tags}" {exec_opts}'.format(**test_args)  # noqa: FS002

    prefixes = [
        "./test/integration/config_providers/...",
        "./test/integration/corechecks/...",
        "./test/integration/listeners/...",
        "./test/integration/util/kubelet/...",
    ]

    for prefix in prefixes:
        ctx.run(f"{go_cmd} {prefix}")


def check_supports_python_version(check_dir, python):
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
        with open(project_file) as f:
            data = toml.loads(f.read())

        project_metadata = data['project']
        if 'requires-python' not in project_metadata:
            return True

        specifier = SpecifierSet(project_metadata['requires-python'])
        # It might be e.g. `>=3.8` which would not immediatelly contain `3`
        for minor_version in range(100):
            if specifier.contains(f'{python}.{minor_version}'):
                return True
        else:
            return False
    elif os.path.isfile(setup_file):
        with open(setup_file) as f:
            tree = ast.parse(f.read(), filename=setup_file)

        prefix = f'Programming Language :: Python :: {python}'
        for node in ast.walk(tree):
            if isinstance(node, ast.keyword) and node.arg == 'classifiers':
                classifiers = ast.literal_eval(node.value)
                return any(cls.startswith(prefix) for cls in classifiers)
        else:
            return False
    else:
        return False


@task
def collect_integrations(_, integrations_dir, python_version, target_os, excluded):
    """
    Collect and print the list of integrations to install.

    `excluded` is a comma-separated list of directories that don't contain an actual integration
    """
    import json

    excluded = excluded.split(',')
    integrations = []

    for entry in os.listdir(integrations_dir):
        int_path = os.path.join(integrations_dir, entry)
        if not os.path.isdir(int_path) or entry in excluded:
            continue

        manifest_file_path = os.path.join(int_path, "manifest.json")

        # If there is no manifest file, then we should assume the folder does not
        # contain a working check and move onto the next
        if not os.path.exists(manifest_file_path):
            continue

        with open(manifest_file_path) as f:
            manifest = json.load(f)

        # Figure out whether the integration is supported on the target OS
        if target_os == 'mac_os':
            tag = 'Supported OS::macOS'
        else:
            tag = f'Supported OS::{target_os.capitalize()}'

        if tag not in manifest['tile']['classifier_tags']:
            continue

        if not check_supports_python_version(int_path, python_version):
            continue

        integrations.append(entry)

    print(' '.join(sorted(integrations)))


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
    cache_version=False,
    pipeline_id=None,
    include_git=True,
    include_pre=True,
    release=False,
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
    if cache_version:
        create_version_json(ctx, git_sha_length=git_sha_length)

    version = get_version(
        ctx,
        include_git=include_git,
        url_safe=url_safe,
        git_sha_length=git_sha_length,
        major_version=major_version,
        include_pipeline_id=True,
        pipeline_id=pipeline_id,
        include_pre=include_pre,
        release=release,
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
def get_integrations_from_cache(ctx, python, bucket, branch, integrations_dir, target_dir, integrations):
    """
    Get cached integration wheels for given integrations.
    python: Python version to retrieve integrations for
    bucket: S3 bucket to retrieve integration wheels from
    branch: namespace in the bucket to get the integration wheels from
    integrations_dir: directory with Git repository of integrations
    target_dir: local directory to put integration wheels to
    integrations: comma-separated names of the integrations to try to retrieve from cache
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
    # WINDOWS NOTES: we have to not put the * in quotes, as there's no expansion on it, unlike on Linux
    exclude_wildcard = "*" if platform.system().lower() == "windows" else "'*'"
    sync_command_prefix = f"{AWS_CMD} s3 sync s3://{bucket} {target_dir} --no-sign-request --exclude {exclude_wildcard}"
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
def upload_integration_to_cache(ctx, python, bucket, branch, integrations_dir, build_dir, integration):
    """
    Upload a built integration wheel for given integration.
    python: Python version the integration is built for
    bucket: S3 bucket to upload the integration wheel to
    branch: namespace in the bucket to upload the integration wheels to
    integrations_dir: directory with Git repository of integrations
    build_dir: directory containing the built integration wheel
    integration: name of the integration being cached
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
    ctx.run(f"{AWS_CMD} s3 cp {wheel_path} s3://{bucket}/{target_name} --acl public-read")


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
