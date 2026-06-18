# mypy: disable-error-code="arg-type"

from invoke import task
from invoke.exceptions import Exit


@task(
    help={
        "check": "Check mode: fail if scenarios_import_gen.go is not up to date (for CI/pre-commit).",
    }
)
def generate_scenario_imports(ctx, check=False):
    """Generate (or verify) test/new-e2e/run/scenarios_import_gen.go.

    Scans test/new-e2e/tests/*/scenario.go and writes a blank-import file
    so each team's init() fires when the demo run binary starts.

    Run after adding a new test/new-e2e/tests/<team>/scenario.go file.
    In CI / pre-commit, pass --check to fail if the file is out of date.
    """
    ctx.run("go run ./tools/generate-scenario-imports/main.go")

    if check:
        result = ctx.run(
            "git diff --exit-code test/new-e2e/run/scenarios_import_gen.go",
            warn=True,
        )
        if result is None or result.exited != 0:
            raise Exit(
                "test/new-e2e/run/scenarios_import_gen.go is out of date.\n"
                "Run: dda inv e2e-framework.generate-scenario-imports",
                code=1,
            )
