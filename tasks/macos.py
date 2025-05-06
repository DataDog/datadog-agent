import time

from invoke import task

from tasks.libs.common.datadog_api import create_count, send_metrics


@task
def report_python_version(ctx):
    """Reports the current Python version to Datadog."""

    version = ctx.run("python3 --version", hide=True).stdout.strip().split()[1].strip()
    global_version = ".".join(version.split(".")[:-1])
    print(f'Reported Python version to Datadog: {version} (global: {global_version})')

    metrics = [
        create_count(
            metric_name='datadog.ci.macos-runners.python-version',
            timestamp=int(time.time()),
            value=1,
            tags=['repository:datadog-agent', f'full-version:{version}', f'global-version:{global_version}'],
        )
    ]
    send_metrics(metrics)


@task
def report_go_version(ctx):
    """Reports the current Go version to Datadog."""

    version = ctx.run("go version", hide=True).stdout.strip().split()[2].strip().removeprefix("go")
    global_version = ".".join(version.split(".")[:-1])
    print(f'Reported Go version to Datadog: {version} (global: {global_version})')

    metrics = [
        create_count(
            metric_name='datadog.ci.macos-runners.go-version',
            timestamp=int(time.time()),
            value=1,
            tags=['repository:datadog-agent', f'full-version:{version}', f'global-version:{global_version}'],
        )
    ]
    send_metrics(metrics)
