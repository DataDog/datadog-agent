from invoke import Context

from tasks.libs.dynamic_test.backend import DynTestBackend, IndexKind
from tasks.libs.dynamic_test.index import DynamicTestIndex


class DynTestExecutor:
    def __init__(self, ctx: Context, kind: IndexKind, backend: DynTestBackend, commit_sha: str):
        self.backend = backend
        self.ctx = ctx
        self.index = self._find_index_for_commit(kind, commit_sha)

    def _find_index_for_commit(self, kind: IndexKind, commit_sha: str) -> DynamicTestIndex:
        closest_commit = ""
        for commit in self.backend.list_indexed_keys(kind):
            is_ancestor = self.ctx.run(f"git merge-base --is-ancestor {commit} {commit_sha}", hide=True)
            if is_ancestor.ok:
                closest_commit = commit
                break
        if closest_commit == "":
            raise RuntimeError(f"No ancestor commit found for {commit_sha}")

        print("Downloading index for commit", closest_commit)
        return self.backend.fetch_index(kind, closest_commit)

    def tests_to_run(self, job_name: str, changes: list[str]) -> list[str]:
        return self.index.impacted_tests(changes, job_name)

    def tests_to_run_per_job(self, changes: list[str]) -> dict[str, list[str]]:
        return self.index.impacted_tests_per_job(changes)
