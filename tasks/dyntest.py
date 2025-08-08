"""
Invoke task to handle dynamic tests.
"""

from invoke import Context, task

from tasks.libs.common.git import get_commit_sha
from tasks.libs.dynamic_test.backend import S3Backend
from tasks.libs.dynamic_test.executor import E2EDynTestExecutor
from tasks.libs.dynamic_test.indexers.e2e import CoverageDynTestIndexer


@task
def compute_and_upload_job_index(ctx: Context, bucket_uri: str, coverage_folder: str, commit_sha: str, job_id: str):
    indexer = CoverageDynTestIndexer(coverage_folder)
    index = indexer.compute_index(ctx)

    uploader = S3Backend(bucket_uri)
    uploader.upload_job_index(index, "e2e", commit_sha, job_id)


@task
def consolidate_index_in_s3(_: Context, bucket_uri: str, commit_sha: str):
    uploader = S3Backend(bucket_uri)
    index = uploader.consolidate_index(commit_sha)

    uploader.upload_full_index(index, commit_sha)


@task
def test(ctx: Context):
    backend = S3Backend("s3://mytestbucketttt-kevinf")

    executor = E2EDynTestExecutor(ctx, backend, get_commit_sha(ctx))
    executor.tests_to_run_per_job()
