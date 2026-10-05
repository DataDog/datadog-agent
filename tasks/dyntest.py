"""
Invoke task to handle dynamic tests.
"""

import os
from time import sleep

from invoke import Context, task
from invoke.exceptions import Exit

from tasks.libs.common.auth import get_aws_vault_env
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_commit_sha, get_modified_files
from tasks.libs.common.utils import environ
from tasks.libs.dynamic_test.backend import S3Backend
from tasks.libs.dynamic_test.evaluator import DatadogDynTestEvaluator
from tasks.libs.dynamic_test.executor import DynTestExecutor
from tasks.libs.dynamic_test.index import IndexKind
from tasks.libs.dynamic_test.indexers.e2e import (
    DiffedPackageCoverageDynTestIndexer,
    FileCoverageDynTestIndexer,
    PackageCoverageDynTestIndexer,
)
from tasks.libs.dynamic_test.jev_selection import JevDynTestExecutor
from tasks.libs.dynamic_test.telemetry import ConsoleTelemetryHandler, DatadogTelemetryHandler
from tasks.new_e2e_tests import DEFAULT_DYNTEST_BUCKET_URI


@task
def compute_and_upload_job_index(ctx: Context, bucket_uri: str, coverage_folder: str, commit_sha: str, job_id: str):
    uploader = S3Backend(bucket_uri)
    run_all_paths = [
        "test/e2e-framework/**/*",  # Modification to the framework should trigger all tests
        "test/new-e2e/go.mod",
        "go.mod",  # incident-47421
        "pkg/config/schema/*",  # DataDog/datadog-agent#52358
        "flakes.yaml",
        "release.json",
        ".gitlab/test/e2e/*.yml",
    ]
    for target in os.getenv("TARGETS").split(","):
        run_all_paths.append(os.path.normpath(os.path.join("test/new-e2e", target) + "/*"))

    # Package coverage indexer
    indexer = PackageCoverageDynTestIndexer(coverage_folder, run_all_paths)
    index_package = indexer.compute_index(ctx)
    uploader.upload_index(index_package, IndexKind.PACKAGE, f"{commit_sha}/{job_id}")

    # File coverage indexer
    indexer = FileCoverageDynTestIndexer(coverage_folder, run_all_paths)
    index_file = indexer.compute_index(ctx)
    uploader.upload_index(index_file, IndexKind.FILE, f"{commit_sha}/{job_id}")

    # Diffed package coverage indexer
    indexer = DiffedPackageCoverageDynTestIndexer(
        coverage_folder, f"{coverage_folder}/testagentbaselinesuite", run_all_changes_paths=run_all_paths
    )
    index_diffed = indexer.compute_index(ctx)
    uploader.upload_index(index_diffed, IndexKind.DIFFED_PACKAGE, f"{commit_sha}/{job_id}")


@task
def consolidate_index_in_s3(_: Context, bucket_uri: str, commit_sha: str):
    uploader = S3Backend(bucket_uri)

    # Package coverage indexer
    index = uploader.consolidate_index(IndexKind.PACKAGE, commit_sha)
    uploader.upload_index(index, IndexKind.PACKAGE, commit_sha)

    # File coverage indexer
    index_file = uploader.consolidate_index(IndexKind.FILE, commit_sha)
    uploader.upload_index(index_file, IndexKind.FILE, commit_sha)

    # Diffed package coverage indexer
    index_diffed = uploader.consolidate_index(IndexKind.DIFFED_PACKAGE, commit_sha)
    uploader.upload_index(index_diffed, IndexKind.DIFFED_PACKAGE, commit_sha)


@task(
    help={
        "bucket-uri": "S3 index bucket (coverage selector only)",
        "commit-sha": "Commit to evaluate; Jev requires it to match HEAD and the pipeline SHA",
        "pipeline-id": "Completed GitLab pipeline ID to evaluate",
        "selector": "coverage (default) or jev",
        "test-env": "CI Visibility environment (defaults to nativetest for Jev, prod for coverage)",
        "lookback-days": "CI Visibility query window in days",
        "send-stats": "Publish evaluation telemetry; use --no-send-stats for local trials",
    }
)
def evaluate_index(
    ctx: Context,
    bucket_uri: str = DEFAULT_DYNTEST_BUCKET_URI,
    commit_sha: str = "",
    pipeline_id: str = "",
    selector: str = "coverage",
    test_env: str = "",
    lookback_days: int = 3,
    send_stats: bool = True,
):
    """Compare a selector's predictions with executed tests using the shared evaluator.

    Coverage evaluates the package/file/diffed-package indexes. Jev evaluates
    completed E2E jobs using their configured targets, without requiring coverage
    data or S3 access. Jev must run from the evaluated pipeline's checkout.

    Requires DD_API_KEY/DD_APP_KEY with CI Visibility read access (and DD_SITE
    when not datadoghq.com). Jev additionally uses the standard GitLab task
    authentication and preinstalled authanywhere for AI Gateway access.
    AI_GATEWAY_TOKEN or JEV_TOKEN_CMD/JEV_DC can override Gateway authentication.
    GITHUB_TOKEN optionally supplies the PR title/description.
    """
    if selector not in {"coverage", "jev"}:
        raise Exit("--selector must be coverage or jev", code=1)
    if not pipeline_id or not pipeline_id.isdecimal() or lookback_days < 1:
        raise Exit("Provide a numeric --pipeline-id and positive --lookback-days", code=1)
    head = get_commit_sha(ctx)
    commit_sha = commit_sha or head
    options = {"test_env": test_env or ("nativetest" if selector == "jev" else "prod"), "lookback_days": lookback_days}
    if selector == "jev":
        if commit_sha != head:
            raise Exit("For Jev, check out the pipeline commit and pass its full SHA (or omit --commit-sha)", code=1)
        executor = JevDynTestExecutor(ctx, commit_sha, pipeline_id)
        executors = [executor]
        options.update(job_ids=executor.job_ids, unreliable_jobs=executor.unreliable_jobs)
        changes = []  # Jev gathers the richer PR diff/context from this checkout.
    else:
        backend = S3Backend(bucket_uri)
        changed_files = get_modified_files(ctx)
        changes = list({os.path.dirname(change) for change in changed_files}) + changed_files
        print("Detected changes:", changed_files)
        executors = [
            DynTestExecutor(ctx, backend, kind, commit_sha)
            for kind in [IndexKind.PACKAGE, IndexKind.FILE, IndexKind.DIFFED_PACKAGE]
        ]

    failed = False
    for i, executor in enumerate(executors):
        if i:
            sleep(10)  # Avoid rate limiting between coverage evaluations.
        telemetry = (
            DatadogTelemetryHandler(
                default_tags=[
                    f"pipeline_id:{pipeline_id}",
                    f"index_kind:{executor.kind.value}",
                    "service:dynamic_test_evaluator",
                ]
            )
            if send_stats
            else ConsoleTelemetryHandler()
        )
        evaluator = DatadogDynTestEvaluator(
            ctx, executor.kind, executor, pipeline_id, telemetry_handler=telemetry, **options
        )
        if not evaluator.initialize():
            print(color_message(f"Failed to initialize the {executor.kind.value} evaluation", Color.RED))
            failed = True
            continue
        results = evaluator.evaluate(changes)
        evaluator.print_summary(results)
        if not results or not any(result.actual_count() for result in results):
            print(
                color_message(
                    "No executed tests found; check the pipeline, --test-env and --lookback-days. No stats sent.",
                    Color.RED,
                )
            )
            failed = True
            continue
        if send_stats:
            evaluator.send_stats_to_datadog(results)
    if failed:
        raise Exit("Evaluation incomplete; see the errors above", code=1)


@task(
    help={
        "job_name": "Name of the CI job containing the test",
        "test_name": "Name of the test to get the triggering path for",
        "index_kind": "Kind of index to use (package, file, diffed_package)",
    }
)
def show_triggering_paths(ctx: Context, job_name: str, test_name: str, index_kind: str = "diffed_package"):
    print(f"Showing triggering path for {test_name} in {job_name} with index kind {index_kind}")
    # Authenticate with aws-vault
    with environ(get_aws_vault_env(ctx, "sso-build-stable-developer")):
        backend = S3Backend(DEFAULT_DYNTEST_BUCKET_URI)
        executor = DynTestExecutor(ctx, backend, IndexKind(index_kind), get_commit_sha(ctx, short=True))
        triggering_path = executor.triggering_paths(job_name, test_name)

    if triggering_path:
        print(f"Triggering paths for {test_name} in {job_name}: {triggering_path}")
    else:
        print(
            f"No triggering path found for {test_name} in {job_name}, it means that the test is in the index, it should never be skipped"
        )
