import time

from invoke import task

from tasks.libs.common.datadog_api import create_count, send_metrics


@task
def report_versions(ctx, lang):
    """Reports the current Python version to Datadog."""

    assert lang in ("all", "python", "go"), "Lang must be either 'all', 'python' or 'go'"

    versions = {}

    if lang in ("all", "python"):
        versions["python"] = ctx.run("python3 --version", hide=True).stdout.strip().split()[1].strip()
    if lang in ("all", "go"):
        versions["go"] = ctx.run("go version", hide=True).stdout.strip().split()[2].strip().removeprefix("go")

    metrics = [
        create_count(
            metric_name=f'datadog.ci.macos-runners.{lang}-version',
            timestamp=int(time.time()),
            value=1,
            tags=[
                'repository:datadog-agent',
                f'full-version:{version}',
                f'global-version:{".".join(version.split(".")[:-1])}',
            ],
        )
        for lang, version in versions.items()
    ]
    send_metrics(metrics)
    print(f'Reported these versions to Datadog: {versions}')
