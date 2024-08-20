from time import sleep
from invoke import task
from invoke.exceptions import Exit

@task
def functional_tests(ctx) -> None:
    with ctx.cd("pkg/collector/corechecks/oracle/compose"):
        ctx.run("docker compose down")
        ctx.run("docker compose rm -f")
        ctx.run("docker compose build")
        ctx.run("docker compose up -d")

        healthy = False
        attempts = 0
        while attempts < 10:
            health_check = ctx.run("docker inspect --format \"{{json .State.Health.Status }}\" compose-oracle-1 | jq", hide=True)
            if health_check.stdout.strip() == '"starting"':
                print("Waiting for oracle to start...")
            elif health_check.stdout.strip() == '"healthy"':
                healthy = True
                break
            attempts += 1
            sleep(3)
        if not healthy:
            ctx.run("docker inspect --format \"{{json .State.Health }}\" compose-oracle-1 | jq")
            raise Exit(message='docker failed to start', code=1)
        
    with ctx.cd("pkg/collector/corechecks/oracle"):
        ctx.run("go test -v -tags test oracle oracle_test ./...")

    with ctx.cd("pkg/collector/corechecks/oracle/compose"):
        ctx.run("docker compose down")
