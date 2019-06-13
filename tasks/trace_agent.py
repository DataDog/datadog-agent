import os
import sys
import shutil

import invoke
from invoke import task

from .utils import bin_name, get_build_flags, get_version_numeric_only, load_release_versions
from .utils import REPO_PATH
from .build_tags import get_build_tags, get_default_build_tags, LINUX_ONLY_TAGS, REDHAT_AND_DEBIAN_ONLY_TAGS, REDHAT_AND_DEBIAN_DIST
from .go import deps

BIN_PATH = os.path.join(".", "bin", "trace-agent")
DEFAULT_BUILD_TAGS = ["netcgo"]

@task
def build(ctx, rebuild=False, race=False, precompile_only=False, use_embedded_libs=False,
          build_include=None, build_exclude=None, puppy=False, use_venv=False):
    """
    Build the trace agent.
    """

    # generate windows resources
    if sys.platform == 'win32':
        ver = get_version_numeric_only(ctx)
        maj_ver, min_ver, patch_ver = ver.split(".")

        ctx.run("windmc --target pe-x86-64 -r cmd/trace-agent/windows_resources cmd/trace-agent/windows_resources/trace-agent-msg.mc")
        ctx.run("windres --define MAJ_VER={maj_ver} --define MIN_VER={min_ver} --define PATCH_VER={patch_ver} -i cmd/trace-agent/windows_resources/trace-agent.rc --target=pe-x86-64 -O coff -o cmd/trace-agent/rsrc.syso".format(
            maj_ver=maj_ver,
            min_ver=min_ver,
            patch_ver=patch_ver
        ))

    ldflags, gcflags, env = get_build_flags(ctx, use_embedded_libs=use_embedded_libs, use_venv=use_venv)
    build_include = DEFAULT_BUILD_TAGS if build_include is None else build_include.split(",")
    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    if puppy:
        # Puppy mode overrides whatever passed through `--build-exclude` and `--build-include`
        build_tags = get_default_build_tags(puppy=True)
    else:
        build_tags = get_build_tags(build_include, build_exclude)

    cmd = "go build {race_opt} {build_type} -tags \"{go_build_tags}\" "
    cmd += "-o {agent_bin} -gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/trace-agent"

    args = {
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else ("-i" if precompile_only else ""),
        "go_build_tags": " ".join(build_tags),
        "agent_bin": os.path.join(BIN_PATH, bin_name("trace-agent", android=False)),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }

    print(env)
    print(cmd.format(**args))
    ctx.run("go generate {REPO_PATH}/pkg/trace/info".format(**args), env=env)
    ctx.run(cmd.format(**args), env=env)

@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False):
    """
    Run integration tests for trace agent
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

    go_cmd = 'INTEGRATION=yes go test {race_opt} -tags "{go_build_tags}" {exec_opts}'.format(**test_args)

    prefixes = [
        "./pkg/trace/test/testsuite/...",
    ]

    for prefix in prefixes:
        print("{} {}".format(go_cmd, prefix))
        ctx.run("{} {}".format(go_cmd, prefix))
