"""
Invoke task to handle dynamic tests.
"""

import os
from time import sleep

from invoke import Context, task

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_modified_files
from tasks.libs.dynamic_test.backend import S3Backend
from tasks.libs.dynamic_test.evaluator import DatadogDynTestEvaluator
from tasks.libs.dynamic_test.executor import DynTestExecutor
from tasks.libs.dynamic_test.index import IndexKind
from tasks.libs.dynamic_test.indexers.e2e import (
    DiffedPackageCoverageDynTestIndexer,
    FileCoverageDynTestIndexer,
    PackageCoverageDynTestIndexer,
)


@task
def compute_and_upload_job_index(ctx: Context, bucket_uri: str, coverage_folder: str, commit_sha: str, job_id: str):
    uploader = S3Backend(bucket_uri)
    run_all_paths = [
        "test/new-e2e/pkg/**/*",  # Modification to the framework should trigger all tests
        "test/new-e2e/go.mod",
        "flakes.yaml",
        "release.json",
        ".gitlab/e2e/e2e.yml",
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
