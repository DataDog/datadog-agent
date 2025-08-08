"""
S3 uploader for Dynamic Test Indexes
"""

import json
import os
import tempfile
from abc import ABC, abstractmethod

from tasks.libs.common.s3 import download_folder_from_s3, list_sorted_keys_in_s3, upload_file_to_s3
from tasks.libs.dynamic_test.index import DynamicTestIndex

DYNAMIC_TEST_PATH = "dynamic_test"


class DynTestBackend(ABC):
    """Abstract base class for Dynamic Test backends."""

    @abstractmethod
    def upload_full_index(self, index: DynamicTestIndex, commit_sha: str) -> str:
        pass

    @abstractmethod
    def upload_job_index(self, index: DynamicTestIndex, job_name: str, commit_sha: str, job_id: str) -> str:
        pass

    @abstractmethod
    def fetch_index(self, commit_sha: str) -> DynamicTestIndex:
        pass

    @abstractmethod
    def list_indexed_commits(self) -> list[str]:
        pass


class S3Backend(DynTestBackend):
    """Uploads DynamicTestIndex artifacts to S3 under a consistent layout.

    Layout:
      <s3_base>/dynamic_test/<short_commit>/full_index.json
      <s3_base>/dynamic_test/<short_commit>/<job_id>/index.json   (per-job index)
    """

    def __init__(self, s3_base_path: str) -> None:
        """
        Args:
            s3_base_path: Base S3 URI (e.g., "s3://my-bucket/prefix") without trailing slash
        """
        self.s3_base_path = s3_base_path.rstrip("/") + "/" + DYNAMIC_TEST_PATH

    def upload_full_index(self, index: DynamicTestIndex, commit_sha: str) -> str:
        """Upload the full consolidated index to S3.

        Args:
            index: DynamicTestIndex to upload
            commit_sha: Full commit SHA (only first 8 chars used in path)

        Returns:
            The destination S3 path used for upload
        """
        short = commit_sha[:8]
        dest_s3_path = f"{self.s3_base_path}/{short}/full_index.json"

        tmp_dir = tempfile.mkdtemp(prefix="dynidx_")
        tmp_file = os.path.join(tmp_dir, "full_index.json")
        index.dump_json(tmp_file)

        upload_file_to_s3(file_path=tmp_file, s3_path=dest_s3_path)
        return dest_s3_path

    def upload_job_index(self, index: DynamicTestIndex, job_name: str, commit_sha: str, job_id: str) -> str:
        """Upload a per-job index file to S3.

        The file contains only the data relevant to the provided job name, encoded as
        { job_name: { package: [tests] } } to match existing consumers.

        Args:
            index: DynamicTestIndex to source data from
            job_name: The job name to extract and upload
            commit_sha: Full commit SHA (only first 8 chars used in path)
            job_id: CI job id used to build upload path

        Returns:
            The destination S3 path used for upload
        """
        job_map = index.get_tests_for_job(job_name)
        payload = {job_name: job_map}

        short = commit_sha[:8]
        dest_s3_path = f"{self.s3_base_path}/{short}/{job_id}/index.json"

        tmp_dir = tempfile.mkdtemp(prefix="dynidx_job_")
        tmp_file = os.path.join(tmp_dir, "index.json")
        with open(tmp_file, "w", encoding="utf-8") as f:
            json.dump(payload, f, indent=2)

        upload_file_to_s3(file_path=tmp_file, s3_path=dest_s3_path)
        return dest_s3_path

    def consolidate_index(self, commit_sha: str) -> DynamicTestIndex:
        # Get system temp folder for downloading index files
        tmp_dir = tempfile.mkdtemp(prefix="dynidx_consolidate_")

        print(f"Downloading index files from {self.s3_base_path}/{commit_sha[:8]}")
        download_folder_from_s3(s3_path=f"{self.s3_base_path}/{commit_sha[:8]}", local_path=tmp_dir)

        consolidated_index = DynamicTestIndex()
        for folder in os.listdir(tmp_dir):
            index_file = os.path.join(tmp_dir, folder, "index.json")
            with open(index_file) as f:
                index = DynamicTestIndex.from_dict(json.load(f))
            consolidated_index.merge(index)

        return consolidated_index

    def fetch_index(self, commit_sha: str) -> DynamicTestIndex:
        tmp_dir = tempfile.mkdtemp(prefix="dynidx_fetch_")
        download_folder_from_s3(s3_path=f"{self.s3_base_path}/{commit_sha[:8]}", local_path=tmp_dir)
        with open(os.path.join(tmp_dir, "full_index.json")) as f:
            return DynamicTestIndex.from_dict(json.load(f))

    def list_indexed_commits(self) -> list[str]:
        keys = list_sorted_keys_in_s3(self.s3_base_path, "full_index.json")
        print(keys)

        return [key.split("/")[0] for key in keys if len(key.split("/")) == 2]
