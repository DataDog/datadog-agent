import os
from time import sleep

from invoke import task
from invoke.exceptions import Exit


@task
def test(ctx, verbose=False) -> None:
    """
    Runs oracle functional tests against a containerized database.
    """

    if not os.environ.get("CI") and not os.environ.get("SKIP_DOCKER"):
        start_docker(ctx, verbose)

    try:
        os.environ["ORACLE_TEST_PORT"] = "1521"
        os.environ["ORACLE_TEST_SERVER"] = "oracle" if os.environ.get("CI") else "localhost"

        with ctx.cd("pkg/collector/corechecks/oracle"):
            print("Running tests...")
            go_flags = " -v" if verbose else ""
            ctx.run(f"go test{go_flags} -tags \"test oracle oracle_test\" ./...")
    finally:
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
        attempts = 0
        while attempts < 120:
            health_check = ctx.run(
                "docker inspect --format \"{{json .State.Health.Status }}\" compose-oracle-1 | jq", hide=True
            )
            if health_check.stdout.strip() == '"starting"':
                dots = ("." * (attempts % 3 + 1)).ljust(3, " ")
                print(f"Waiting for oracle to be ready{dots}", end="\r")
            elif health_check.stdout.strip() == '"healthy"':
                healthy = True
                break
            attempts += 1
            sleep(1)
        print()
        if not healthy:
            ctx.run("docker inspect --format \"{{json .State.Health }}\" compose-oracle-1 | jq")
            ctx.run("docker logs compose-oracle-1")
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
