"""
S3 backend for Dynamic Test Indexes
"""

import json
import os
import tempfile
from abc import ABC, abstractmethod

from tasks.libs.common.s3 import (
    download_file_from_s3,
    download_folder_from_s3,
    list_sorted_keys_in_s3,
    upload_file_to_s3,
)
from tasks.libs.dynamic_test.index import DynamicTestIndex, IndexKind

DYNAMIC_TEST_PATH = "dynamic_test"


class DynTestBackend(ABC):
    """Abstract base class for Dynamic Test backends."""

    @abstractmethod
    def upload_index(self, index: DynamicTestIndex, kind: IndexKind, key: str) -> str:
        pass

    @abstractmethod
    def fetch_index(self, kind: IndexKind, key: str) -> DynamicTestIndex:
        pass

    @abstractmethod
    def list_indexed_keys(self, kind: IndexKind) -> list[str]:
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

    def upload_index(self, index: DynamicTestIndex, kind: IndexKind, key: str) -> str:
        """Upload  index to S3.

        Args:
            index: DynamicTestIndex to upload
            kind: Kind of index to upload
            key: Key to upload the index under
        Returns:
            The destination S3 path used for upload
        """
        dest_s3_path = f"{self.s3_base_path}/{kind.value}/{key}/index.json"

        tmp_dir = tempfile.mkdtemp(prefix="dynidx_")
        tmp_file = os.path.join(tmp_dir, "index.json")
        index.dump_json(tmp_file)

        upload_file_to_s3(file_path=tmp_file, s3_path=dest_s3_path)
        return dest_s3_path

    def consolidate_index(self, kind: IndexKind, key: str) -> DynamicTestIndex:
        # Get system temp folder for downloading index files
        tmp_dir = tempfile.mkdtemp(prefix="dynidx_consolidate_")

        print(f"Downloading index files from {self.s3_base_path}/{kind.value}/{key}")
        download_folder_from_s3(s3_path=f"{self.s3_base_path}/{kind.value}/{key}", local_path=tmp_dir)

        consolidated_index = DynamicTestIndex()
        for folder in os.listdir(tmp_dir):
            if not os.path.isdir(os.path.join(tmp_dir, folder)):
                continue
            index_file = os.path.join(tmp_dir, folder, "index.json")
            with open(index_file) as f:
                index = DynamicTestIndex.from_dict(json.load(f))
            consolidated_index.merge(index)

        return consolidated_index

    def fetch_index(self, kind: IndexKind, key: str) -> DynamicTestIndex:
        tmp_dir = tempfile.mkdtemp(prefix="dynidx_fetch_")
        download_file_from_s3(
            s3_path=f"{self.s3_base_path}/{kind.value}/{key}/index.json", local_path=f"{tmp_dir}/index.json"
        )
        with open(os.path.join(tmp_dir, "index.json")) as f:
            return DynamicTestIndex.from_dict(json.load(f))

    def list_indexed_keys(self, kind: IndexKind) -> list[str]:
        keys = list_sorted_keys_in_s3(self.s3_base_path, f"{kind.value}/index.json")

        return [key.split("/")[0] for key in keys if len(key.split("/")) == 3]
