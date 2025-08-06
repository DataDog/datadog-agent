import json
import os
import tempfile

from invoke import Context

from tasks.gotest import get_modified_packages
from tasks.libs.common.s3 import download_folder_from_s3, list_sorted_keys_in_s3


class DynTestExecutor:
    def __init__(self, s3_path: str):
        self.s3_path = s3_path

    def _retrieve_index(self, ctx: Context, current_commit_sha: str):
        uploaded_indexes = list_sorted_keys_in_s3(self.s3_path, "full_index.json")
        most_recent_ancestor = ""
        for uploaded_index in uploaded_indexes:
            split_index = uploaded_index.split("/")
            if len(split_index) != 2:
                continue
            commit = split_index[0]
            # Check if the uploaded commit is an ancestor of the current commit
            result = ctx.run(f"git merge-base --is-ancestor {commit} {current_commit_sha}", hide=True)
            if result.ok:
                # Found an ancestor commit
                most_recent_ancestor = commit
                break
        if most_recent_ancestor == "":
            raise RuntimeError(f"No ancestor commit found for {current_commit_sha}")
        with tempfile.TemporaryDirectory() as tmp_dir:
            download_folder_from_s3(f"{self.s3_path}/{most_recent_ancestor}/full_index.json", tmp_dir)
            with open(os.path.join(tmp_dir, "full_index.json")) as f:
                self.index = json.load(f)


class E2EDynTestExecutor(DynTestExecutor):
    def __init__(self, s3_path: str):
        super().__init__(s3_path)

    def _get_modified_packages(self, ctx: Context) -> list[str]:
        modified_packages_per_module = get_modified_packages(ctx)
        modified_packages = []
        for module, packages in modified_packages_per_module.items():
            for package in packages:
                modified_packages.append(os.path.normpath(os.path.join(module, package)))
        return modified_packages

    def get_tests_to_run(self, ctx: Context, job_name: str) -> list[str]:
        modified_packages = self._get_modified_packages(ctx)
        impacted_tests = []
        for package in modified_packages:
            if package in self.index[job_name]:
                impacted_tests.extend(self.index[job_name][package])
        return impacted_tests


