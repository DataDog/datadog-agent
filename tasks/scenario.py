"""
`dda lab` forwarder: builds the scenariorun binary in the test/e2e-framework Go
module and forwards arguments to it. Mirrors tasks/ai_sandbox.py.
"""

from __future__ import annotations

import os

from invoke.tasks import task

E2E_FRAMEWORK_DIR = "test/e2e-framework"
CLI_PACKAGE = "./cmd/scenariorun"
CLI_BIN = "bin/scenariorun"


@task(
    auto_shortflags=False,
    help={"args": "Arguments forwarded verbatim to the scenariorun binary"},
)
def lab(ctx, args=""):
    """Build (if needed) and run the scenariorun CLI, forwarding ARGS.

    Example: dda inv lab --args="create ec2-host --os debian-12"
    """
    # `go build -o` does not create the output's parent directory, and bin/ is
    # gitignored (absent in a clean checkout), so create it first.
    os.makedirs(os.path.join(E2E_FRAMEWORK_DIR, os.path.dirname(CLI_BIN)), exist_ok=True)
    with ctx.cd(E2E_FRAMEWORK_DIR):
        ctx.run(f"go build -o {CLI_BIN} {CLI_PACKAGE}")
        ctx.run(f"./{CLI_BIN} {args}", pty=True)
