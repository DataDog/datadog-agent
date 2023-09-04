"""
All-in-one namespaced tasks
"""


import os
import sys

from invoke import task
from invoke.exceptions import Exit

from .agent import render_config
from .security_agent import render_config as render_security_agent_config
from .build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from .flavor import AgentFlavor
from .rtloader import install as rtloader_install
from .rtloader import make as rtloader_make
from .utils import (
    REPO_PATH,
    bin_name,
    get_build_flags,
)

BIN_DIR = os.path.join(".", "bin")
BIN_PATH = os.path.join(BIN_DIR, "allinone", bin_name("allinone"))

ALLINONE_AGENTS = ["agent", "process-agent", "trace-agent", "security-agent", "system-probe"]


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
        raise Exit("allinone is only supported on Linux")

    if flavor.is_iot():
        raise Exit("allinone cannot be built for iot flavor")

    all_tags = set()
    if development:
        all_tags.add("ebpf_bindata")

    for build in ALLINONE_AGENTS:
        include = (
            get_default_build_tags(build=build, arch=arch, flavor=flavor)
            if build_include is None
            else filter_incompatible_tags(build_include.split(","), arch=arch)
        )

        exclude = [] if build_exclude is None else build_exclude.split(",")
        build_tags = get_build_tags(include, exclude)

        all_tags |= set(build_tags)
    build_tags = list(all_tags)

    cmd = "go build -mod={go_mod} {race_opt} {build_type} -tags \"{go_build_tags}\" "

    cmd += "-o {allinone_bin} -gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/allinone"
    args = {
        "go_mod": go_mod,
        "race_opt": "-race" if race else "",
        "build_type": "-a" if rebuild else "",
        "go_build_tags": " ".join(build_tags),
        "allinone_bin": BIN_PATH,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
        "flavor": "iot-agent" if flavor.is_iot() else "agent",
    }
    ctx.run(cmd.format(**args), env=env)

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

    render_security_agent_config(
        ctx,
        env=env,
        skip_assets=skip_assets
    )
