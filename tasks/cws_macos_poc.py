"""
Build the macOS Workload Protection collector proof of concept.

The collector reads Endpoint Security events from /usr/bin/eslogger, evaluates
them against SECL rules, and ships matches straight to the runtime-security
intake. See the Workload-Protection-on-macOS RFC (CWS-6817).
"""

from __future__ import annotations

import os
import sys

from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags

BIN_DIR = os.path.join(".", "bin", "cws-macos-poc")
BIN_PATH = os.path.join(BIN_DIR, bin_name("cws-macos-poc"))

# The logs pipeline compresses payloads before POSTing them to the intake, and
# the compressor is selected at build time. Without these tags the binary links
# pkg/util/compression/selector/no-zlib-no-zstd.go, which logs "invalid
# compression set" and every payload fails to send -- a failure that looks like a
# network problem rather than a build-tag problem.
#
# This is the minimal subset of SECURITY_AGENT_TAGS the PoC needs: docker, ec2,
# netcgo and the WAF tags describe environments a developer laptop is not in.
BUILD_TAGS = ["zlib", "zstd"]


@task
def build(ctx, race=False, rebuild=False, go_mod="readonly"):
    """
    Build the macOS CWS PoC collector.
    """
    if sys.platform != "darwin":
        raise Exit(
            message="cws-macos-poc only builds on macOS: it depends on /usr/bin/eslogger "
            "and on darwin-only sources.",
            code=1,
        )

    ldflags, gcflags, env = get_build_flags(ctx)

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/cws-macos-poc",
        mod=go_mod,
        race=race,
        rebuild=rebuild,
        gcflags=gcflags,
        ldflags=ldflags,
        build_tags=BUILD_TAGS,
        bin_path=BIN_PATH,
        env=env,
    )
