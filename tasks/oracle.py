import os
from time import monotonic, sleep

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.utils import join_command
from tasks.schema.generate import schema_codegen


@task
def test(ctx, verbose=False) -> None:
    """
    Runs oracle functional tests against a containerized database.
    """

    manage_docker = not os.environ.get("CI") and not os.environ.get("SKIP_DOCKER")
    try:
        if manage_docker:
            start_docker(ctx, verbose)
        env = {
            "ORACLE_TEST_PORT": os.environ.get("ORACLE_TEST_PORT", "1521"),
            "ORACLE_TEST_SERVER": os.environ.get(
                "ORACLE_TEST_SERVER", "oracle" if os.environ.get("CI") else "localhost"
            ),
        }

        # TODO: remove once Bazel is used to build the Agent
        schema_codegen(ctx)

        with ctx.cd("pkg/collector/corechecks/oracle"):
            print("Running tests...")
            go_flags = " -v" if verbose else ""
            ctx.run(f"go test{go_flags} -count=1 -timeout=20m -tags \"test oracle oracle_test\" ./...", env=env)
    finally:
        if manage_docker:
            clean(ctx, verbose)


@task
def start_docker(ctx, verbose=False) -> None:
    """
    Starts a local oracle instance in docker. Used when running individual oracle tests.
    """

    # Start a local oracle instance
    with ctx.cd("pkg/collector/corechecks/oracle/compose"):
        print("Launching docker...")
        ctx.run("docker compose down", hide=not verbose)
        ctx.run("docker compose rm -f", hide=not verbose)
        ctx.run("docker compose build", hide=not verbose)
        ctx.run("docker compose up -d", hide=not verbose)

        healthy = False
        container_id = ctx.run("docker compose ps -q oracle", hide=True).stdout.strip()
        if not container_id:
            raise Exit(message="Oracle container was not created", code=1)
        deadline = monotonic() + 600
        while monotonic() < deadline:
            health_check = ctx.run(
                join_command(
                    ["docker", "inspect", "--format", "{{.State.Status}} {{.State.Health.Status}}", container_id]
                ),
                hide=True,
            )
            status = health_check.stdout.strip()
            if status == "running healthy":
                healthy = True
                break
            if status != "running starting":
                break
            print("Waiting for oracle to be ready...", end="\r")
            sleep(1)
        print()
        if not healthy:
            ctx.run(join_command(["docker", "inspect", "--format", "{{json .State}}", container_id]), warn=True)
            ctx.run(join_command(["docker", "logs", container_id]), warn=True)
            raise Exit(message='docker failed to start', code=1)


@task
def clean(ctx, verbose=False) -> None:
    """
    Stops the local oracle instance in docker.
    """
    print("Cleaning up...")
    if not os.environ.get("CI"):
        with ctx.cd("pkg/collector/corechecks/oracle/compose"):
            ctx.run("docker compose down", hide=not verbose)
