import os

from invoke import Context

from tasks.gotest import get_modified_packages
from tasks.libs.dynamic_test.backend import DynTestBackend
from tasks.libs.dynamic_test.index import DynamicTestIndex


class DynTestExecutor:
    def __init__(self, ctx: Context, backend: DynTestBackend, commit_sha: str):
        self.backend = backend
        self._index = self._find_index_for_commit(commit_sha)
        self.ctx = ctx

    def _find_index_for_commit(self, commit_sha: str) -> DynamicTestIndex:
        closest_commit = ""
        for commit in self.backend.list_indexed_commits():
            is_ancestor = self.ctx.run(f"git merge-base --is-ancestor {commit} {commit_sha}", hide=True)
            if is_ancestor.ok:
                closest_commit = commit
                break
        if closest_commit == "":
            raise RuntimeError(f"No ancestor commit found for {commit_sha}")
        return self.backend.fetch_index(closest_commit)


class E2EDynTestExecutor(DynTestExecutor):
    def __init__(self, ctx: Context, s3_path: str, commit_sha: str):
        super().__init__(ctx, s3_path, commit_sha)
        self._modified_packages = self._get_modified_packages(self.ctx)

    def _get_modified_packages(self, ctx: Context) -> list[str]:
        modified_packages_per_module = get_modified_packages(ctx)
        modified_packages = []
        for module in modified_packages_per_module:
            for package in module.test_targets:
                modified_packages.append(os.path.normpath(os.path.join(module.path, package)))

        return modified_packages

    def tests_to_run(self, job_name: str) -> list[str]:
        return self._index.impacted_tests(self._modified_packages, job_name)

    def tests_to_run_per_job(self) -> dict[str, list[str]]:
        return self._index.impacted_packages_per_job(self._modified_packages)
