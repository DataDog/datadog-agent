"""
Invoke task to handle dynamic tests.
"""

import os

from invoke import Context, task

from tasks.libs.common.git import get_modified_files
from tasks.libs.dynamic_test.backend import S3Backend
from tasks.libs.dynamic_test.evaluator import DatadogDynTestEvaluator
from tasks.libs.dynamic_test.executor import DynTestExecutor
from tasks.libs.dynamic_test.index import IndexKind
from tasks.libs.dynamic_test.indexers.e2e import CoverageDynTestIndexer


@task
def compute_and_upload_job_index(ctx: Context, bucket_uri: str, coverage_folder: str, commit_sha: str, job_id: str):
    indexer = CoverageDynTestIndexer(coverage_folder)
    index = indexer.compute_index(ctx)
    uploader = S3Backend(bucket_uri)
    uploader.upload_index(index, IndexKind.PACKAGE, f"{commit_sha}/{job_id}")


@task
def consolidate_index_in_s3(_: Context, bucket_uri: str, commit_sha: str):
    uploader = S3Backend(bucket_uri)
    index = uploader.consolidate_index(IndexKind.PACKAGE, commit_sha)
    uploader.upload_index(index, IndexKind.PACKAGE, commit_sha)


@task
def evaluate_index(ctx: Context, bucket_uri: str, commit_sha: str, pipeline_id: str):
    changes = get_modified_files(ctx)

    uploader = S3Backend(bucket_uri)
    executor = DynTestExecutor(ctx, IndexKind.PACKAGE, uploader, commit_sha)
    evaluator = DatadogDynTestEvaluator(ctx, IndexKind.PACKAGE, executor, pipeline_id)

    results = evaluator.evaluate([os.path.dirname(change) for change in changes])
    evaluator.print_summary(results)
    evaluator.send_stats_to_datadog(results)
