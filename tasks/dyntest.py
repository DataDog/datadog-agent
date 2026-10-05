"""
Invoke task to handle dynamic tests.
"""

import os
from time import sleep

from invoke import Context, task

from tasks.libs.common.auth import get_aws_vault_env
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_commit_sha, get_modified_files
from tasks.libs.common.utils import environ
from tasks.libs.dynamic_test.backend import S3Backend
from tasks.libs.dynamic_test.evaluator import DatadogDynTestEvaluator
from tasks.libs.dynamic_test.executor import DynTestExecutor
from tasks.libs.dynamic_test.index import IndexKind
from tasks.libs.dynamic_test.jev_selection import JevDynTestExecutor
from tasks.libs.dynamic_test.telemetry import DatadogTelemetryHandler
from tasks.libs.dynamic_test.indexers.e2e import (
    DiffedPackageCoverageDynTestIndexer,
    FileCoverageDynTestIndexer,
    PackageCoverageDynTestIndexer,
)
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


@task
def evaluate_index(ctx: Context, bucket_uri: str, commit_sha: str, pipeline_id: str):
    uploader = S3Backend(bucket_uri)

    def evaluate(kind: IndexKind, changes: list[str]):
        executor = DynTestExecutor(ctx, uploader, kind, commit_sha)
        evaluator = DatadogDynTestEvaluator(ctx, kind, executor, pipeline_id)
        if not evaluator.initialize():
            print(color_message(f"WARNING: Failed to initialize index for {kind.value} coverage", Color.ORANGE))
            return
        results = evaluator.evaluate(changes)
        evaluator.print_summary(results)
        evaluator.send_stats_to_datadog(results)

    changed_files = get_modified_files(ctx)
    changed_packages = list({os.path.dirname(change) for change in changed_files})
    print("Detected changes:", changed_files)

    for kind in [IndexKind.PACKAGE, IndexKind.FILE, IndexKind.DIFFED_PACKAGE]:
        evaluate(kind, changed_packages + changed_files)
        sleep(10)  # small sleep to avoid rate limiting


@task(
    help={
        "pipeline-id": "CI pipeline ID to evaluate against",
    }
)
def evaluate_jev_index(ctx, pipeline_id):
    """Evaluate the accuracy of the Jev-based test skipping with the same evaluator as the coverage index.

    Uses DynTestEvaluator with a Jev-backed executor. Unlike the coverage
    evaluation, the test universe is NOT restricted to the tests present in
    the coverage index: every e2e job that ran in the pipeline is evaluated
    (GitLab API), and every test entry point under test/new-e2e/tests is
    decidable - including tests without coverage data or brand new tests.
    not_executed_failing_count is the miss count (executed, failing, and not
    predicted); stats are tagged selector:jev.

    Requires DD_SITE/DD_API_KEY/DD_APP_KEY (CI Visibility), the GitLab token
    (authanywhere in CI, GITLAB_TOKEN or ddtool locally) and the AI Gateway
    token (authanywhere in CI, JEV_TOKEN_CMD/JEV_DC locally).
    """
    jev_executor = JevDynTestExecutor(ctx, None, IndexKind.DIFFED_PACKAGE, get_commit_sha(ctx, short=True), pipeline_id)
    evaluator = DatadogDynTestEvaluator(
        ctx,
        IndexKind.DIFFED_PACKAGE,
        jev_executor,
        pipeline_id,
        telemetry_handler=DatadogTelemetryHandler(
            default_tags=[
                f"pipeline_id:{pipeline_id}",
                "index_kind:diffed_package",
                "service:dynamic_test_evaluator",
                "selector:jev",
                "universe:all-e2e-tests",
            ]
        ),
    )
    if not evaluator.initialize():
        print(color_message("WARNING: Failed to initialize the Jev test universe", Color.ORANGE))
        return

    changed_files = get_modified_files(ctx)
    print("Detected changes:", changed_files)

    results = evaluator.evaluate(changed_files)
    evaluator.print_summary(results)
    evaluator.send_stats_to_datadog(results)

    # Sanity check: a vacuous evaluation (jobs evaluated, but zero executed
    # tests found overall) is invisible in the metrics above and would look
    # like a perfect selector with zero misses everywhere. If this fires, the
    # executed-set queries matched no events - check the env tag of the e2e
    # test events against the evaluator's query.
    total_actual = sum(r.actual_count() for r in results)
    if results and total_actual == 0:
        print(
            color_message(
                "WARNING: no executed tests found for ANY of the "
                f"{len(results)} evaluated jobs - the evaluation is vacuous. "
                "The executed-test queries likely matched no CI Visibility events "
                "(env tag mismatch: the e2e jobs tag their events env:nativetest).",
                Color.RED,
            )
        )


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
