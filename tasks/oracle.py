import os
from time import sleep

from invoke import task
from invoke.exceptions import Exit


@task
def test(ctx, verbose=False) -> None:
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

    try:
        ctx.run("docker ps")
        os.environ["ORACLE_TEST_PORT"] = "1521"
        if os.environ.get("CI"):            
            os.environ["ORACLE_TEST_SERVER"] = ctx.run(
            "docker inspect  -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' compose-oracle-1", hide=True
        ).stdout.strip()
        ctx.run("echo port=$ORACLE_TEST_PORT server=$ORACLE_TEST_SERVER")
        with ctx.cd("pkg/collector/corechecks/oracle"):
            print("Running tests...")
            go_flags = " -v" if verbose else ""
            ctx.run(f"go test{go_flags} -tags \"test oracle oracle_test\" ./...")
    finally:
        clean(ctx, verbose)


@task
def clean(ctx, verbose=False) -> None:
    print("Cleaning up...")
    with ctx.cd("pkg/collector/corechecks/oracle/compose"):
        ctx.run("docker compose down", hide=not verbose)
