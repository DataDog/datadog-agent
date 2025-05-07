import time

from invoke import task

from tasks.libs.common.datadog_api import create_count, send_metrics


@task
def report_versions(ctx, lang):
    """Reports the current Python version to Datadog."""

    assert lang in ("python", "go"), "Lang must be either 'python' or 'go'"

    if lang == "python":
        version = ctx.run("python3 --version", hide=True).stdout.strip().split()[1].strip()
    elif lang == "go":
        version = ctx.run("go version", hide=True).stdout.strip().split()[2].strip().removeprefix("go")
    global_version = ".".join(version.split(".")[:-1])

    metrics = [
        create_count(
            metric_name=f'datadog.ci.macos-runners.{lang}-version',
            timestamp=int(time.time()),
            value=1,
            tags=['repository:datadog-agent', f'full-version:{version}', f'global-version:{global_version}'],
        )
    ]
    send_metrics(metrics)
    print(f'Reported {lang} version to Datadog: {version} (global: {global_version})')
