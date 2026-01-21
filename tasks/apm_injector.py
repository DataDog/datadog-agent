"""
apm-injector namespaced tasks
"""

import sys
from os import getenv, path

from invoke import task

from tasks.build_tags import (
    compute_build_tags_for_flavor,
)
from tasks.flavor import AgentFlavor
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags
from tasks.windows_resources import build_messagetable, build_rc, versioninfo_vars

DIR_BIN = path.join(".", "bin", "apm-injector")
APM_INJECTOR_BIN = path.join(DIR_BIN, bin_name("apm-injector"))


@task
def build(
    ctx,
    output_bin=None,
    rebuild=False,
    race=False,
    install_path=None,
    run_path=None,
    build_include=None,
    build_exclude=None,
    go_mod="readonly",
    no_strip_binary=True,
    no_cgo=False,
    fips_mode=False,
):
    """
    Build the apm-injector.
    """

    ldflags, gcflags, env = get_build_flags(ctx, install_path=install_path, run_path=run_path)

    if sys.platform == 'win32':
        build_messagetable(ctx)
        vars = versioninfo_vars(ctx)
        build_rc(
            ctx,
            "cmd/apm-injector/windows_resources/datadog-apm-injector.rc",
            vars=vars,
            out="cmd/apm-injector/rsrc.syso",
        )

    build_tags = compute_build_tags_for_flavor(
        build="apm-injector",
        build_include=build_include,
        build_exclude=build_exclude,
        flavor=AgentFlavor.fips if fips_mode else AgentFlavor.base,
    )

    apm_injector_bin = APM_INJECTOR_BIN
    if output_bin:
        apm_injector_bin = output_bin

    if no_cgo and not fips_mode:
        env["CGO_ENABLED"] = "0"
    else:
        env["CGO_ENABLED"] = "1"

    if not no_strip_binary:
        ldflags += " -s -w"

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/apm-injector",
        mod=go_mod,
        race=race,
        rebuild=rebuild,
        gcflags=gcflags,
        ldflags=ldflags,
        build_tags=build_tags,
        bin_path=apm_injector_bin,
        check_deadcode=getenv("DEPLOY_AGENT") == "true",
        env=env,
    )
