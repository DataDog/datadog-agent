"""
Invoke task to handle dynamic tests.
"""

import json
import os
from time import sleep

from invoke import Context, task
from invoke.exceptions import Exit

from tasks.libs.common.auth import get_aws_vault_env
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.feature_flags import is_enabled
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
from tasks.libs.dynamic_test.jev_selection import (
    JOB_CANDIDATES_FILE,
    JevDynTestExecutor,
    generate_job_candidates,
)
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
        "commit-sha": "Commit to evaluate; Jev requires it to match HEAD",
        "pipeline-id": "Completed GitLab pipeline ID to evaluate",
        "selector": "coverage (default) or jev",
        "send-stats": "Publish evaluation telemetry; use --no-send-stats for local trials",
    }
)
def evaluate_index(
    ctx: Context,
    bucket_uri: str = DEFAULT_DYNTEST_BUCKET_URI,
    commit_sha: str = "",
    pipeline_id: str = "",
    selector: str = "coverage",
    send_stats: bool = True,
):
    """Compare a selector's predictions with executed tests using the shared evaluator.

    Coverage evaluates the package/file/diffed-package indexes. Jev evaluates
    the ENTIRE committed job -> test candidates file
    (tasks/libs/dynamic_test/jev/job_test_candidates.json) - every job it
    knows, whether or not that job ran in the pipeline - against the
    pipeline's executed tests: the evaluation therefore also shows which
    tests and jobs Jev would run but the pipeline did not (over-selection),
    without requiring coverage data or S3 access. Jev must run from the
    evaluated pipeline's checkout.

    Requires DD_API_KEY/DD_APP_KEY with CI Visibility read access (and DD_SITE
    when not datadoghq.com). Jev additionally uses preinstalled authanywhere
    for AI Gateway access.
    AI_GATEWAY_TOKEN or JEV_TOKEN_CMD/JEV_DC can override Gateway authentication.
    GITHUB_TOKEN optionally supplies the PR title/description.

    The Jev evaluation is gated by the 'datadog-agent-jev-evaluation'
    feature flag: disabled, the task exits 0 without evaluating.
    """
    if selector not in {"coverage", "jev"}:
        raise Exit("--selector must be coverage or jev", code=1)
    if not pipeline_id or not pipeline_id.isdecimal():
        raise Exit("Provide a numeric --pipeline-id", code=1)
    head = get_commit_sha(ctx)
    commit_sha = commit_sha or head
    executors: list[DynTestExecutor] = []
    if selector == "jev":
        if not is_enabled(ctx, "datadog-agent-jev-evaluation"):
            print(color_message("Jev evaluation disabled", Color.ORANGE))
            return
        if commit_sha != head:
            raise Exit("For Jev, check out the pipeline commit and pass its full SHA (or omit --commit-sha)", code=1)
        # A plain DynTestExecutor with a static index (committed in Git, where
        # the coverage executors keep theirs in S3). The shared evaluator owns
        # the CI Visibility queries.
        executor = JevDynTestExecutor(ctx, commit_sha)
        executors = [executor]
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
        evaluator = DatadogDynTestEvaluator(ctx, executor.kind, executor, pipeline_id, telemetry_handler=telemetry)
        if not evaluator.initialize():
            print(
                color_message(
                    f"Failed to initialize the {executor.kind.value} evaluation: {evaluator.initialization_error}",
                    Color.RED,
                )
            )
            failed = True
            continue
        results = evaluator.evaluate(changes)
        evaluator.print_summary(results)
        if not results or not any(result.actual_count() for result in results):
            print(
                color_message(
                    "No executed tests found; check the pipeline (executed tests are queried over the last 3 days). No stats sent.",
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


@task(
    help={
        "pipeline-id": "Completed pipeline to read the executed tests from (repeatable; queries a 30-day window, env:nativetest - the e2e jobs' tag)",
    },
    iterable=["pipeline_id"],
)
def generate_jev_job_index(ctx: Context, pipeline_id):
    """(Re)generate the committed Jev job -> test candidates file.

    A job's candidates are the tests that executed in it, unioned across the
    given pipelines (one pipeline-wide CI Visibility query each, latest job
    attempts only, over the last 30 days). Pick pipelines where the e2e jobs
    ran the widest (full-suite main pipelines or the largest dev pipelines)
    for the most complete map: a test the coverage selection skipped in every
    given pipeline stays missing from its job's candidates. The file does not
    need to track CI changes - jobs missing from it are skipped from the Jev
    evaluation with a report, never a failure.

    Requires DD_API_KEY/DD_APP_KEY (org 2, CI Visibility read access).
    Commit the result.
    """
    candidates = generate_job_candidates(pipeline_id)
    data = {job: sorted(tests) for job, tests in sorted(candidates.items())}
    JOB_CANDIDATES_FILE.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
    print(f"[jev] {len(data)} jobs -> {JOB_CANDIDATES_FILE}")
