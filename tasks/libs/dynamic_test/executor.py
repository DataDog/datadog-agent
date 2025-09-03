from invoke import Context

from tasks.libs.dynamic_test.backend import DynTestBackend, IndexKind
from tasks.libs.dynamic_test.index import DynamicTestIndex


class DynTestExecutor:
    """Executes dynamic test selection using a lazily-loaded stored index.

    The executor uses lazy loading to find and load indexes on-demand:
    1. Initialize with backend, commit SHA, and index kind (no index loaded yet)
    2. Index is automatically loaded when first accessed via index() method
    3. Uses the index to determine impacted tests for specific changes

    This class bridges the gap between stored indexes and actual test execution,
    handling the complexity of finding appropriate historical indexes when the
    exact commit doesn't have one available.

    Typical workflow:
    1. Initialize with backend, index kind, and commit SHA
    2. Call tests_to_run() or tests_to_run_per_job() with changed packages
    3. Index is automatically loaded on first use
    4. Execute the returned tests in your CI system

    Lazy Loading Benefits:
    - Allows evaluator to handle index loading errors gracefully
    - Defers expensive index loading until actually needed
    - Enables better error reporting and monitoring
    """

    def __init__(self, ctx: Context, backend: DynTestBackend, kind: IndexKind, commit_sha: str):
        """Initialize the executor with lazy index loading.

        The index is not loaded during initialization but will be loaded
        automatically when first accessed via the index() method or when
        calling tests_to_run() methods.

        Args:
            ctx: Invoke context for running git commands
            backend: Backend to load the index from
            kind: Type of index to load (e.g., IndexKind.PACKAGE)
            commit_sha: Target commit SHA to find an index for

        Note:
            No exceptions are raised during initialization. Index loading
            errors will be raised when the index is first accessed.
        """
        self.backend = backend
        self.ctx = ctx
        self.commit_sha = commit_sha
        self.kind = kind
        self._index = None

    def index(self) -> DynamicTestIndex:
        """Get the loaded index, loading it lazily if needed.

        Returns:
            DynamicTestIndex: The loaded index for this executor

        Raises:
            RuntimeError: If no ancestor commit with an index is found
            Exception: If index loading fails due to backend issues
        """
        if self._index is None:
            self.init_index()
        return self._index

    def init_index(self):
        """Load the index from the backend.

        Finds the most recent ancestor commit with an available index
        and loads it from the backend.

        Raises:
            RuntimeError: If no ancestor commit with an index is found
            Exception: If backend operations fail
        """
        self._index = self._find_index_for_commit(self.kind, self.commit_sha)

    def _find_index_for_commit(self, kind: IndexKind, commit_sha: str) -> DynamicTestIndex:
        """Find and load the best available index for the given commit.

        Searches through available indexed commits to find the most recent ancestor
        of the target commit that has an available index.

        Args:
            kind: Type of index to search for
            commit_sha: Target commit to find an index for

        Returns:
            DynamicTestIndex: The loaded index from the closest ancestor commit

        Raises:
            RuntimeError: If no ancestor commit with an index is found
        """
        closest_commit = ""
        for commit in self.backend.list_indexed_keys(kind):
            is_ancestor = self.ctx.run(f"git merge-base --is-ancestor {commit} {commit_sha}", hide=True, warn=True)
            if is_ancestor.ok:
                closest_commit = commit
                break
        if closest_commit == "":
            raise RuntimeError(f"No ancestor commit found for {commit_sha}")

        print("Downloading index for commit", closest_commit)
        return self.backend.fetch_index(kind, closest_commit)

    def tests_to_run(self, job_name: str, changes: list[str]) -> set[str]:
        """Determine which tests should be executed for a specific job and changes.

        Args:
            job_name: Name of the CI job to get tests for
            changes: List of modified package/component names

        Returns:
            set[str]: Set of test names that should be executed
        """

        return self.index().impacted_tests(changes, job_name)

    def tests_to_run_per_job(self, changes: list[str]) -> dict[str, set[str]]:
        """Determine which tests should be executed across all jobs for given changes.

        Args:
            changes: List of modified package/component names

        Returns:
            dict[str, set[str]]: Mapping of job names to sets of tests that should be executed
        """

        return self.index().impacted_tests_per_job(changes)
