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
    """Abstract base class for Dynamic Test backends.

    Defines the interface for storing, retrieving, and managing dynamic test indexes.
    Implementations should provide concrete storage mechanisms (e.g., S3, local filesystem).

    The backend is responsible for:
    - Persisting DynamicTestIndex objects with proper organization
    - Retrieving indexes by kind and key
    - Listing available indexed keys for discovery

    Expected storage organization:
    - Indexes should be organized by kind (IndexKind enum values)
    - Within each kind, indexes are keyed (typically by commit SHA)
    - Multiple job-specific indexes may exist under the same key
    """

    @abstractmethod
    def upload_index(self, index: DynamicTestIndex, kind: IndexKind, key: str) -> str:
        """Upload a DynamicTestIndex to the backend storage.

        The implementation should:
        1. Serialize the index to a persistent format (typically JSON)
        2. Store it in a location determined by kind and key
        3. Return the storage location/identifier for confirmation

        Args:
            index: The DynamicTestIndex to upload
            kind: The type of index (from IndexKind enum)
            key: Unique identifier for this index (typically commit SHA)

        Returns:
            str: The backend-specific path or identifier where the index was stored

        Raises:
            Exception: If upload fails due to storage issues or serialization errors
        """
        pass

    @abstractmethod
    def fetch_index(self, kind: IndexKind, key: str) -> DynamicTestIndex:
        """Retrieve a DynamicTestIndex from the backend storage.

        The implementation should:
        1. Locate the stored index using kind and key
        2. Download/read the serialized index data
        3. Deserialize and return a DynamicTestIndex object

        Args:
            kind: The type of index to fetch (from IndexKind enum)
            key: Unique identifier for the index (typically commit SHA)

        Returns:
            DynamicTestIndex: The deserialized index object

        Raises:
            FileNotFoundError: If no index exists for the given kind and key
            Exception: If retrieval or deserialization fails
        """
        pass

    @abstractmethod
    def list_indexed_keys(self, kind: IndexKind) -> list[str]:
        """List all available keys for a given index kind.

        Used for discovering what indexes are available, typically for finding
        the most recent ancestor commit that has an available index.

        The implementation should:
        1. Query the backend for all stored indexes of the specified kind
        2. Extract and return the unique keys (typically commit SHAs)
        3. Return keys in a consistent order (preferably chronological)

        Args:
            kind: The type of index to list keys for (from IndexKind enum)

        Returns:
            list[str]: List of unique keys available for the specified kind.
                      Typically commit SHAs, sorted by recency or alphabetically.

        Note:
            The returned keys should be suitable for use with fetch_index()
            and upload_index() methods.
        """
        pass


class S3Backend(DynTestBackend):
    """S3-based implementation of DynTestBackend.

    Stores DynamicTestIndex artifacts in S3 using a hierarchical layout that
    organizes indexes by type and commit, with support for both consolidated
    and per-job indexes.

    S3 Layout:
        <s3_base>/dynamic_test/<index_kind>/<commit_sha>/index.json
        <s3_base>/dynamic_test/<index_kind>/<commit_sha>/<job_id>/index.json

    Features:
    - Automatic temporary file management for uploads/downloads
    - Support for consolidating multiple job-specific indexes
    - Efficient key listing with proper filtering

    The backend handles:
    - JSON serialization/deserialization of indexes
    - Temporary directory creation and cleanup
    - S3 path construction following the expected layout
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
        keys = list_sorted_keys_in_s3(f"{self.s3_base_path}/{kind.value}", "index.json")

        return [key.split("/")[0] for key in keys if len(key.split("/")) == 2]
