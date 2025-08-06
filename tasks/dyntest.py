"""
Invoke task to handle dynamic tests.
"""

import os

from invoke import Context, task

from tasks.libs.dynamic_test.uploader import CoverageDynTestUploader, consolidate_index


@task
def compute_and_upload_index(ctx: Context):
    if "E2E_COVERAGE_OUT_DIR" not in os.environ:
        print("E2E_COVERAGE_OUT_DIR is not set")
        return
    if "S3_PERMANENT_ARTIFACTS_URI" not in os.environ:
        print("S3_PERMANENT_ARTIFACTS_URI is not set")
        return

    coverage_folder = os.environ["E2E_COVERAGE_OUT_DIR"]
    s3_path = os.environ["S3_PERMANENT_ARTIFACTS_URI"]
    uploader = CoverageDynTestUploader(s3_path, coverage_folder)
    index_file, metadata_file = uploader.compute_index(ctx)
    uploader.upload_index(ctx, index_file, metadata_file)


@task
def consolidate_index_in_s3(ctx: Context, commit_sha: str, s3_uri: str):
    consolidate_index(ctx, commit_sha, s3_uri)
