"""
Generate test/e2e-framework/docs/CONFIG.md, the reference of every Pulumi
config key read by the e2e framework.

This wraps the standalone `cmd/configdoc` binary in the `test/e2e-framework`
Go module (a separate go.mod from the root datadog-agent module).
"""

from __future__ import annotations

from invoke.tasks import task

E2E_FRAMEWORK_DIR = "test/e2e-framework"
CLI_PACKAGE = "./cmd/configdoc"


@task(
    auto_shortflags=False,
    help={"check": "Verify docs/CONFIG.md is up to date instead of regenerating it (used in CI)"},
)
def configdoc(ctx, check=False):
    """
    Regenerate test/e2e-framework/docs/CONFIG.md from the Pulumi config keys
    declared in common/config/, resources/*/environment.go, and other files
    that open a config namespace (e.g. run/main.go, scenarios/aws/gensim-eks).

    Example:
        dda inv e2e.configdoc            # regenerate and write the file
        dda inv e2e.configdoc --check    # fail if the file is out of date
    """
    with ctx.cd(E2E_FRAMEWORK_DIR):
        ctx.run(f"go run {CLI_PACKAGE}{' -check' if check else ''}")
