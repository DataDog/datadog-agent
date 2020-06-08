import os
import sys
import shutil

import invoke
from invoke import task

from .utils import bin_name, get_build_flags, get_version_numeric_only, load_release_versions
from .utils import REPO_PATH
from .build_tags import get_build_tags, get_default_build_tags, LINUX_ONLY_TAGS
from .go import deps

BIN_PATH = os.path.join(".", "bin", "trace-agent")

DEFAULT_BUILD_TAGS = [
    "netcgo",
    "secrets",
    "docker",
    "kubeapiserver",
    "kubelet",
]

@task
def build(ctx, rebuild=False, race=False, precompile_only=False, build_include=None,
          build_exclude=None, major_version='7', python_runtimes='3', arch="x64", go_mod="vendor"):
    """
    Build the trace agent.
    """

    # get env prior to windows sources so we only have to set the target architecture once
    ldflags, gcflags, env = get_build_flags(ctx, arch=arch, major_version=major_version, python_runtimes=python_runtimes)

    # generate windows resources
    if sys.platform == 'win32':
        windres_target = "pe-x86-64"
        if arch == "x86":
            env["GOARCH"] = "386"
            windres_target = "pe-i386"

        ver = get_version_numeric_only(ctx, env, major_version=major_version)
        maj_ver, min_ver, patch_ver = ver.split(".")

        ctx.run("windmc --target {target_arch}  -r cmd/trace-agent/windows_resources cmd/trace-agent/windows_resources/trace-agent-msg.mc".format(target_arch=windres_target))
        ctx.run("windres --define MAJ_VER={maj_ver} --define MIN_VER={min_ver} --define PATCH_VER={patch_ver} -i cmd/trace-agent/windows_resources/trace-agent.rc --target {target_arch} -O coff -o cmd/trace-agent/rsrc.syso".format(
            maj_ver=maj_ver,
            min_ver=min_ver,
            patch_ver=patch_ver,
            target_arch=windres_target
        ))


    build_include = DEFAULT_BUILD_TAGS if build_include is None else build_include.split(",")
    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    if not sys.platform.startswith('linux'):
        for ex in LINUX_ONLY_TAGS:
            if ex not in build_exclude:
                build_exclude.append(ex)

    build_tags = get_build_tags(build_include, build_exclude)

    cmd = "go build -mod={go_mod} {race_opt} {build_type} -tags \"{go_build_tags}\" "
    cmd += "-o {agent_bin} -gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/trace-agent"

    args = {
        "go_mod": go_mod,
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else "",
        "go_build_tags": " ".join(build_tags),
        "agent_bin": os.path.join(BIN_PATH, bin_name("trace-agent", android=False)),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }

    ctx.run("go generate -mod={go_mod} {REPO_PATH}/pkg/trace/info".format(**args), env=env)
    ctx.run(cmd.format(**args), env=env)

@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False, go_mod="vendor"):
    """
    Run integration tests for trace agent
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

    go_cmd = 'INTEGRATION=yes go test -mod={go_mod} {race_opt} -v'.format(**test_args)

    prefixes = [
        "./pkg/trace/test/testsuite/...",
    ]

    for prefix in prefixes:
        ctx.run("{} {}".format(go_cmd, prefix))

@task
def cross_compile(ctx, tag=""):
    """
    Cross-compiles the trace-agent binaries. Use the "--tag=X" argument to specify build tag.
    """
    if not tag:
        print("Argument --tag=<version> is required.")
        return

    print("Building tag %s..." % tag)

    env = {
        "TRACE_AGENT_VERSION": tag,
        "V": tag,
    }

    ctx.run("git checkout $V", env=env)
    ctx.run("mkdir -p ./bin/trace-agent/$V", env=env)
    ctx.run("go generate -mod=vendor ./pkg/trace/info", env=env)
    ctx.run("go get -u github.com/karalabe/xgo")
    ctx.run("xgo -dest=bin/trace-agent/$V -go=1.11 -out=trace-agent-$V -targets=windows-6.1/amd64,linux/amd64,darwin-10.11/amd64 ./cmd/trace-agent", env=env)
    ctx.run("mv ./bin/trace-agent/$V/trace-agent-$V-windows-6.1-amd64.exe ./bin/trace-agent/$V/trace-agent-$V-windows-amd64.exe", env=env)
    ctx.run("mv ./bin/trace-agent/$V/trace-agent-$V-darwin-10.11-amd64 ./bin/trace-agent/$V/trace-agent-$V-darwin-amd64 ", env=env)
    ctx.run("git checkout -")

    print("Done! Binaries are located in ./bin/trace-agent/%s" % tag)
