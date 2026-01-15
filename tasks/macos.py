import glob
import os
import time

from invoke import task

from tasks.libs.common.color import Color, color_message
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


@task
def list_ci_active_versions(_, lang, n_days=30):
    """Lists the Python / Go versions reported to Datadog during the last month."""

    assert lang in ("python", "go"), "Lang must be either 'python' or 'go'"

    from datetime import datetime, timedelta

    from datadog_api_client import ApiClient, Configuration
    from datadog_api_client.v1.api.metrics_api import MetricsApi

    with ApiClient(Configuration(enable_retry=True)) as api_client:
        api_instance = MetricsApi(api_client)
        response = api_instance.query_metrics(
            _from=int((datetime.now() + timedelta(days=-n_days)).timestamp()),
            to=int(datetime.now().timestamp()),
            query=f"sum:datadog.ci.macos_runners.{lang}_version{{*}} by {{global-version}}.as_count()",
        )

        versions = set()
        for series in response['series']:
            version = series['scope'].split(':')[1]
            versions.add(version)

        if versions:
            print(f'CI active {lang} versions:')
            for version in sorted(versions):
                print(version)
        else:
            print(f'No ci active {lang} versions found')

    return versions


@task
def list_runner_active_versions(ctx, lang):
    """Lists the Python / Go versions on this runner."""

    assert lang in ("python", "go"), "Lang must be either 'python' or 'go'"

    if lang == "python":
        versions = {
            v.strip()
            for v in ctx.run(
                "ls ~/.pyenv/versions | grep -E '^[0-9]+\\.[0-9]+\\.[0-9]+$'", hide=True
            ).stdout.splitlines()
        }
    elif lang == "go":
        versions = {
            # goX.Y.Z.darwin.<arch>
            # or goX.Y.darwin.<arch>
            # to X.Y or X.Y.Z
            '.'.join(v.strip().removeprefix('go').split('.')[:-2])
            for v in ctx.run("ls ~/.gimme/versions | grep -E '^go[0-9]+\\.[0-9]+'", hide=True).stdout.splitlines()
        }

    if versions:
        print(f'Runner {lang} versions:')
        for version in sorted(versions):
            print(version)
    else:
        print(f'No runner {lang} versions found')

    return versions


@task
def remove_inactive_versions(ctx, lang, target_version="", n_days=30, dry_run=False):
    """Removes the Python / Go versions that have not been reported to Datadog during the last month."""

    assert lang in ("python", "go"), "Lang must be either 'python' or 'go'"

    # These are X.YY versions
    ci_active_versions = list_ci_active_versions(ctx, lang, n_days)
    # These are X.YY.ZZ versions
    runner_active_versions = list_runner_active_versions(ctx, lang)

    # Avoid removing everything if metrics are not found
    if not ci_active_versions:
        print(f'{color_message("WARNING", Color.ORANGE)}: No versions metrics found in Datadog, skipping removal')
        return

    # Transform target version from go<version> to <version>
    if lang == "go":
        target_version = target_version.removeprefix("go")

    # Transform target version from X.YY.ZZ to X.YY
    if target_version.count('.') == 2:
        target_version = '.'.join(target_version.split('.')[:-1])

    # Remove all versions that are not in the CI (except the target version)
    for version in runner_active_versions:
        for ci_version in ci_active_versions:
            if version.startswith(ci_version):
                break
        else:
            if not target_version or not version.startswith(target_version):
                # This version is not in the CI and is not the target one
                print(f'Removing {lang} version {version}')
                if not dry_run:
                    if lang == "python":
                        ctx.run(f"pyenv uninstall -f {version}")
                    elif lang == "go":
                        install_dir = glob.glob(f"{os.environ['HOME']}/.gimme/versions/go{version}*")
                        assert len(install_dir) == 1, f"Expected one Go version for {version}: {install_dir}"
                        ctx.run(f"rm -rf {install_dir}")
