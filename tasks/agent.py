"""
Agent namespaced tasks
"""

import glob
import os
import platform
import re
import shutil
import sys
import tempfile

from invoke import task
from invoke.exceptions import Exit

from tasks import core_checks, doc
from tasks.build_tags import (
    compute_build_tags_for_flavor,
    get_default_build_tags,
)
from tasks.devcontainer import run_on_devcontainer
from tasks.flavor import AgentFlavor
from tasks.gointegrationtest import (
    CORE_AGENT_WINDOWS_IT_CONF,
    containerized_integration_tests,
)
from tasks.libs.common.constants import CONTAINER_PLATFORM_MAPPING
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import (
    REPO_PATH,
    _resolve_platform,
    bin_name,
    get_build_flags,
    get_version,
    gitlab_section,
)
from tasks.libs.releasing.version import create_version_json
from tasks.rtloader import clean as rtloader_clean
from tasks.rtloader import install as rtloader_install
from tasks.rtloader import install_with_bazel as rtloader_install_with_bazel
from tasks.rtloader import make as rtloader_make
from tasks.schema.generate import compress as schema_compress
from tasks.schema.template import CORE_SCHEMA_FILE, SYSPROBE_SCHEMA_FILE, generate_template
from tasks.windows_resources import build_messagetable, build_rc, versioninfo_vars

# constants
BIN_DIR = os.path.join(".", "bin")
BIN_PATH = os.path.join(BIN_DIR, "agent")
AGENT_TAG = "datadog/agent:master"


@task
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
    python_home_3=None,
    exclude_rtloader=False,
    go_mod="readonly",
    windows_sysprobe=False,
    cmake_options='',
    agent_bin=None,
    run_on=None,  # noqa: U100, F841. Used by the run_on_devcontainer decorator
    glibc=True,
    enable_bazel=True,
):
    """
    Build the agent. If the bits to include in the build are not specified,
    the values from `invoke.yaml` will be used.

    Bazel-backed build steps are enabled by default.
    Use `--no-enable-bazel` to keep the legacy build paths.

    Example invokation:
        dda inv agent.build --build-exclude=systemd
    """
    flavor = AgentFlavor[flavor]
    target_platform = _resolve_target_platform()

    if not exclude_rtloader and not flavor.is_iot() and target_platform != "aix":
        # On AIX, rtloader is built natively in advance as a prerequisite.
        with gitlab_section("Install embedded rtloader", collapsed=True):
            if enable_bazel:
                bazel_embedded = rtloader_install_with_bazel(ctx)
                embedded_path = bazel_embedded
                python_home_3 = bazel_embedded
            else:
                rtloader_make(ctx, install_prefix=embedded_path, cmake_options=cmake_options)
                rtloader_install(ctx)

    if flavor.is_iot():
        # Iot mode overrides whatever passed through `--build-exclude` and `--build-include`
        build_tags = get_default_build_tags(build="agent", flavor=flavor)
    else:
        build_tags = compute_build_tags_for_flavor(
            build="agent",
            flavor=flavor,
            build_include=build_include,
            build_exclude=build_exclude,
            platform=target_platform,
        )

    if not glibc:
        build_tags = list(set(build_tags).difference({"nvml"}))

    ldflags, gcflags, env = get_build_flags(
        ctx,
        install_path=install_path,
        embedded_path=embedded_path,
        rtloader_root=rtloader_root,
        python_home_3=python_home_3,
        include_python="python" in build_tags,
        platform=target_platform,
    )

    if target_platform == 'win32':
        # Important for x-compiling
        env["CGO_ENABLED"] = "1"

        build_messagetable(ctx)
        # Do not call build_rc when cross-compiling on Linux as the intend is more
        # to streamline the development process that producing a working executable / installer
        if sys.platform == 'win32':
            vars = versioninfo_vars(ctx)
            build_rc(
                ctx,
                "cmd/agent/windows_resources/agent.rc",
                vars=vars,
                out="cmd/agent/rsrc.syso",
            )

    if not agent_bin:
        agent_bin = os.path.join(BIN_PATH, bin_name("agent"))

    flavor_cmd = "iot-agent" if flavor.is_iot() else "agent"

    # AIX build hosts do not have bazel; the compressed schema files are
    # committed to the repo and do not need regeneration there.
    if sys.platform != "aix":
        schema_compress(ctx)

    with gitlab_section("Build agent", collapsed=True):
        go_build(
            ctx,
            f"{REPO_PATH}/cmd/{flavor_cmd}",
            mod=go_mod,
            env=env,
            bin_path=agent_bin,
            race=race,
            rebuild=rebuild,
            gcflags=gcflags,
            ldflags=ldflags,
            build_tags=build_tags,
            check_deadcode=os.getenv("DEPLOY_AGENT") == "true",
            coverage=os.getenv("E2E_COVERAGE_PIPELINE") == "true",
        )

    with gitlab_section("Generate configuration files", collapsed=True):
        generate_config_examples(
            ctx,
            flavor=flavor,
            skip_assets=skip_assets,
            build_tags=build_tags,
            development=development,
            windows_sysprobe=windows_sysprobe,
        )


_PLATFORM_TO_OS_TARGET = {
    "linux": "linux",
    "win32": "windows",
    "darwin": "darwin",
    "aix": "aix",
}


def generate_config_examples(ctx, flavor, skip_assets, build_tags, development, windows_sysprobe):
    os_target = _PLATFORM_TO_OS_TARGET[sys.platform]

    build_type = "iot-agent" if flavor.is_iot() else "agent-py3"
    generate_template(CORE_SCHEMA_FILE, "./cmd/agent/dist/datadog.yaml", build_type, os_target)

    if sys.platform != 'win32' or windows_sysprobe:
        generate_template(SYSPROBE_SCHEMA_FILE, "./cmd/agent/dist/system-probe.yaml", "system-probe", os_target)

    if not skip_assets:
        refresh_assets(ctx, build_tags, development=development, flavor=flavor.name, windows_sysprobe=windows_sysprobe)


@task
def refresh_assets(_, build_tags, development=True, flavor=AgentFlavor.base.name, windows_sysprobe=False):
    """
    Clean up and refresh Collector's assets and config files
    """
    flavor = AgentFlavor[flavor]
    # ensure BIN_PATH exists (makedirs handles missing parents, e.g. on AIX build hosts)
    os.makedirs(BIN_PATH, exist_ok=True)

    dist_folder = os.path.join(BIN_PATH, "dist")
    if os.path.exists(dist_folder):
        shutil.rmtree(dist_folder)
    os.mkdir(dist_folder)

    if "python" in build_tags:
        shutil.copytree(
            "./cmd/agent/dist/checks/",
            os.path.join(dist_folder, "checks"),
            ignore=shutil.ignore_patterns("BUILD.bazel"),
            dirs_exist_ok=True,
        )
        shutil.copytree(
            "./cmd/agent/dist/utils/",
            os.path.join(dist_folder, "utils"),
            ignore=shutil.ignore_patterns("BUILD.bazel"),
            dirs_exist_ok=True,
        )
        shutil.copy("./cmd/agent/dist/config.py", os.path.join(dist_folder, "config.py"))
    if not flavor.is_iot():
        shutil.copy("./cmd/agent/dist/dd-agent", os.path.join(dist_folder, "dd-agent"))
        # copy the dd-agent placeholder to the bin folder
        bin_ddagent = os.path.join(BIN_PATH, "dd-agent")
        shutil.move(os.path.join(dist_folder, "dd-agent"), bin_ddagent)

    # System probe not supported on windows
    if sys.platform != 'win32' or windows_sysprobe:
        shutil.copy("./cmd/agent/dist/system-probe.yaml", os.path.join(dist_folder, "system-probe.yaml"))
    shutil.copy("./cmd/agent/dist/datadog.yaml", os.path.join(dist_folder, "datadog.yaml"))

    if sys.platform.startswith('aix'):
        checks_to_copy = core_checks.AIX_CORECHECKS
    elif flavor.is_iot():
        checks_to_copy = core_checks.IOT_AGENT_CORECHECKS
    else:
        checks_to_copy = core_checks.AGENT_CORECHECKS
    for check in checks_to_copy:
        check_dir = os.path.join(dist_folder, f"conf.d/{check}.d/")
        shutil.copytree(
            f"./cmd/agent/dist/conf.d/{check}.d/",
            check_dir,
            ignore=shutil.ignore_patterns("BUILD.bazel"),
            dirs_exist_ok=True,
        )
        # Ensure the config folders are not world writable
        os.chmod(check_dir, mode=0o755)

    # add additional windows-only corechecks, only on windows. Otherwise the check loader
    # on linux will throw an error because the module is not found, but the config is.
    if sys.platform == 'win32':
        for check in core_checks.WINDOWS_CORECHECKS:
            check_dir = os.path.join(dist_folder, f"conf.d/{check}.d/")
            shutil.copytree(
                f"./cmd/agent/dist/conf.d/{check}.d/",
                check_dir,
                ignore=shutil.ignore_patterns("BUILD.bazel"),
                dirs_exist_ok=True,
            )

    if sys.platform == 'darwin':
        shutil.copy("./cmd/agent/dist/conf.d/apm.yaml.default", os.path.join(dist_folder, "conf.d/apm.yaml.default"))
        shutil.copy(
            "./cmd/agent/dist/conf.d/process_agent.yaml.default",
            os.path.join(dist_folder, "conf.d/process_agent.yaml.default"),
        )

    shutil.copytree(
        "./comp/core/gui/impl/views/private",
        os.path.join(dist_folder, "views"),
        ignore=shutil.ignore_patterns("BUILD.bazel"),
        dirs_exist_ok=True,
    )
    if development:
        shutil.copytree("./dev/dist/", dist_folder, ignore=shutil.ignore_patterns("BUILD.bazel"), dirs_exist_ok=True)


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
    enable_bazel=True,
):
    """
    Execute the agent binary.

    By default it builds the agent before executing it, unless --skip-build was
    passed. It accepts the same set of options as agent.build.
    """
    if not skip_build:
        build(ctx, rebuild, race, build_include, build_exclude, flavor, enable_bazel=enable_bazel)

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

    This works against an agent run via `dda inv agent.run`.
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
def image_build(ctx, arch='amd64', base_dir="omnibus", skip_tests=False, tag=None, push=False):
    """
    Build the docker image
    """
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

    # Build with the testing target
    if not skip_tests:
        ctx.run(f"docker build {common_build_opts} --platform linux/{arch} --target testing {build_context}")

    # Build with the release target
    ctx.run(f"docker build {common_build_opts} --platform linux/{arch} --target release {build_context}")
    if push:
        ctx.run(f"docker push {tag}")

    ctx.run(f"rm {build_context}/{deb_glob}")


@task(
    help={
        "base_image": doc.base_image,
        "target_image": doc.target_image,
        "process_agent": doc.process_agent,
        "trace_agent": doc.trace_agent,
        "system_probe": doc.system_probe,
        "security_agent": doc.security_agent,
        "trace_loader": doc.trace_loader,
        "privateactionrunner": doc.privateactionrunner,
        "push": doc.push,
        "race": doc.race,
        "signed_pull": doc.signed_pull,
        "arch": doc.arch,
        "development": doc.development,
    }
)
def hacky_dev_image_build(
    ctx,
    base_image=None,
    target_image="agent",
    process_agent=False,
    trace_agent=False,
    system_probe=False,
    security_agent=False,
    trace_loader=False,
    privateactionrunner=False,
    push=False,
    race=False,
    signed_pull=False,
    arch=None,
    development=True,
    build_exclude=None,
):
    """
    Builds the agent or cluster-agent Docker image.
    """
    if arch is None:
        arch = CONTAINER_PLATFORM_MAPPING.get(platform.machine().lower())

    if arch is None:
        print("Unable to determine architecture to build, please set `arch`", file=sys.stderr)
        raise Exit(code=1)

    if base_image is None:
        import requests
        import semver

        # Try to guess what is the latest release of the agent
        latest_release = semver.VersionInfo(0)
        tags = requests.get("https://registry.datadoghq.com/v2/agent/tags/list", timeout=10)
        for tag in tags.json()['tags']:
            if not semver.VersionInfo.isvalid(tag):
                continue
            ver = semver.VersionInfo.parse(tag)
            if ver.prerelease or ver.build:
                continue
            if ver > latest_release:
                latest_release = ver
        base_image = f"registry.datadoghq.com/agent:{latest_release}"

    # Extract the python library of the docker image
    with tempfile.TemporaryDirectory() as extracted_python_dir:
        ctx.run(
            f"docker run --platform linux/{arch} --rm '{base_image}' bash -c 'tar --create /opt/datadog-agent/embedded/{{bin,lib,include}}/*python*' | tar --directory '{extracted_python_dir}' --extract"
        )

        if development:
            os.environ["DELVE"] = "1"
        os.environ["LD_LIBRARY_PATH"] = (
            os.environ.get("LD_LIBRARY_PATH", "") + f":{extracted_python_dir}/opt/datadog-agent/embedded/lib"
        )
        build(
            ctx,
            race=race,
            development=development,
            build_exclude=build_exclude,
            cmake_options=f'-DPython3_ROOT_DIR={extracted_python_dir}/opt/datadog-agent/embedded -DPython3_FIND_STRATEGY=LOCATION',
            enable_bazel=False,
        )
        ctx.run(
            f'perl -0777 -pe \'s|{extracted_python_dir}(/opt/datadog-agent/embedded/lib/python\\d+\\.\\d+/../..)|substr $1."\\0"x length$&,0,length$&|e or die "pattern not found"\' -i dev/lib/libdatadog-agent-three.so'
        )

    copy_checks_d = ""
    copy_checks_d_final = ""
    if sys.platform.startswith("linux"):
        from tasks.rust_shared_checks import build as rust_shared_checks_build

        checks_d_staging = "bin/agent/dist/checks.d"
        rust_shared_checks_build(ctx, checks_d_dir=checks_d_staging)
        if os.path.isdir(checks_d_staging) and any(
            f.startswith("libdatadog-agent-") for f in os.listdir(checks_d_staging)
        ):
            copy_checks_d = f"COPY {checks_d_staging} /etc/datadog-agent/checks.d\n"
            copy_checks_d_final = "COPY --from=bin /etc/datadog-agent/checks.d /etc/datadog-agent/checks.d\n"

    copy_extra_agents = ""
    if security_agent:
        from tasks.security_agent import build as security_agent_build

        security_agent_build(ctx, [""])
        copy_extra_agents += "COPY bin/security-agent/security-agent /opt/datadog-agent/embedded/bin/security-agent\n"

    if process_agent:
        from tasks.process_agent import build as process_agent_build

        process_agent_build(ctx)
        copy_extra_agents += "COPY bin/process-agent/process-agent /opt/datadog-agent/embedded/bin/process-agent\n"

    if trace_agent:
        from tasks.trace_agent import build as trace_agent_build

        trace_agent_build(ctx)
        copy_extra_agents += "COPY bin/trace-agent/trace-agent /opt/datadog-agent/embedded/bin/trace-agent\n"

    if trace_loader:
        from tasks.loader import build as trace_loader_build

        trace_loader_build(ctx)
        copy_extra_agents += "COPY bin/trace-loader/trace-loader /opt/datadog-agent/embedded/bin/trace-loader\n"

    if privateactionrunner:
        from tasks.privateactionrunner import build as privateactionrunner_build

        privateactionrunner_build(ctx)
        copy_extra_agents += (
            "COPY bin/privateactionrunner/privateactionrunner /opt/datadog-agent/embedded/bin/privateactionrunner\n"
        )

    copy_ebpf_assets = ""
    copy_ebpf_assets_final = ""
    if system_probe:
        from tasks.libs.types.arch import Arch
        from tasks.system_probe import build as system_probe_build
        from tasks.system_probe import get_ebpf_build_dir, get_ebpf_runtime_dir

        system_probe_build(ctx)

        build_arch = Arch.from_str(arch)
        build_dir = get_ebpf_build_dir(build_arch)
        runtime_dir = get_ebpf_runtime_dir()

        copy_extra_agents += (
            "COPY bin/system-probe/system-probe /opt/datadog-agent/embedded/bin/system-probe\n"
            "COPY pkg/discovery/module/rust/embedded/bin/system-probe-lite /opt/datadog-agent/embedded/bin/system-probe-lite\n"
        )
        copy_ebpf_assets = f"""
RUN mkdir -p /opt/datadog-agent/embedded/share/system-probe/ebpf/co-re/
RUN mkdir -p /opt/datadog-agent/embedded/share/system-probe/ebpf/runtime/
COPY {build_dir}/*.o         /opt/datadog-agent/embedded/share/system-probe/ebpf/
COPY {build_dir}/co-re/*.o   /opt/datadog-agent/embedded/share/system-probe/ebpf/co-re/
COPY {runtime_dir}/*.c       /opt/datadog-agent/embedded/share/system-probe/ebpf/runtime/
"""
        copy_ebpf_assets_final = """
COPY --from=bin /opt/datadog-agent/embedded/share/system-probe/ebpf /opt/datadog-agent/embedded/share/system-probe/ebpf
"""

    with tempfile.NamedTemporaryFile(mode='w') as dockerfile:
        dockerfile.write(
            f'''FROM ubuntu:latest AS src

COPY . /usr/src/datadog-agent

RUN find /usr/src/datadog-agent -type f \\! -name \\*.go -print0 | xargs -0 rm
RUN find /usr/src/datadog-agent -type d -empty -print0 | xargs -0 rmdir

FROM ubuntu:latest AS bin

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get clean && \
    apt-get -o Acquire::Retries=4 update && \
    apt-get install -y patchelf

COPY bin/agent/agent                            /opt/datadog-agent/bin/agent/agent
COPY bin/agent/dist/conf.d                      /etc/datadog-agent/conf.d
{copy_checks_d}
COPY dev/lib/libdatadog-agent-rtloader.so.0.1.0 /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0
COPY dev/lib/libdatadog-agent-three.so          /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so
{copy_ebpf_assets}

RUN patchelf --set-rpath /opt/datadog-agent/embedded/lib /opt/datadog-agent/bin/agent/agent
RUN patchelf --set-rpath /opt/datadog-agent/embedded/lib /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0
RUN patchelf --set-rpath /opt/datadog-agent/embedded/lib /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so

FROM golang:latest AS dlv

RUN go install github.com/go-delve/delve/cmd/dlv@v1.26.0

FROM {base_image} AS bash_completion

RUN apt-get clean && \
    apt-get -o Acquire::Retries=4 update && \
    apt-get install -y gawk

RUN awk -i inplace '!/^#/ {{uncomment=0}} uncomment {{gsub(/^#/, "")}} /# enable bash completion/ {{uncomment=1}} {{print}}' /etc/bash.bashrc

FROM {base_image}

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get clean && \
    apt-get -o Acquire::Retries=4 update && \
    apt-get install -y bash-completion less vim tshark && \
    apt-get clean

ENV DELVE_PAGER=less

COPY --from=dlv /go/bin/dlv /usr/local/bin/dlv
COPY --from=bash_completion /etc/bash.bashrc /etc/bash.bashrc
COPY --from=src /usr/src/datadog-agent {os.getcwd()}
COPY --from=bin /opt/datadog-agent/bin/agent/agent                                 /opt/datadog-agent/bin/agent/agent
COPY --from=bin /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0 /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0
COPY --from=bin /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so          /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so
COPY --from=bin /etc/datadog-agent/conf.d /etc/datadog-agent/conf.d
{copy_checks_d_final}
{copy_extra_agents}
{copy_ebpf_assets_final}
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
        ctx.run(f'docker build --platform linux/{arch} -t {target_image} -f {dockerfile.name} .', env=pull_env)

        if push:
            ctx.run(f'docker push {target_image}')


@task
def integration_tests(ctx, race=False, go_mod="readonly", timeout=""):
    """
    Run integration tests for the Agent
    """
    if sys.platform == 'win32':
        return containerized_integration_tests(
            ctx, CORE_AGENT_WINDOWS_IT_CONF, race=race, go_mod=go_mod, timeout=timeout
        )


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
        #   Moreover, of the two rules, we actually really only use the first one (because we always use dda inv agent.version --url-safe).
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


@task()
def build_remote_agent(ctx, env=None):
    """
    Builds the remote-agent example client.
    """
    return go_build(ctx, "./internal/remote-agent", verbose=True, bin_path="bin/remote-agent", env=env or {})
