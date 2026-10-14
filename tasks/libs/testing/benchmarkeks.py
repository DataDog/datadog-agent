"""
Baseline resolution of the benchmarkeks e2e suite (TestBenchmarkEKSSuite in test/new-e2e/tests/containers).

The suite compares the resource usage of the Agent of the pipeline under test with the one of a baseline Agent. The
baseline commit is chosen like the SMP regression detector does (.gitlab/test/functional_test/regression_detector.yml):
the merge base with the base branch, or the parent commit on the base branch, walking first parents back to the first
commit with images.

The images of the agent-qa ECR repositories e2e tests pull from only live for a day, so the baseline images are looked
up in the registry the container build jobs push to, which keeps them for months, to be published to agent-qa again.
"""

from __future__ import annotations

import re
from collections import defaultdict
from collections.abc import Callable, Iterable, Mapping, Sequence
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta

from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_common_ancestor
from tasks.libs.common.utils import join_command

# Repositories the container build jobs push the images of every pipeline to, tagged `v<pipeline ID>-<short SHA>-...`
SOURCE_AGENT_REPOSITORY = "registry.ddbuild.io/ci/datadog-agent/agent"
SOURCE_CLUSTER_AGENT_REPOSITORY = "registry.ddbuild.io/ci/datadog-agent/cluster-agent"
# Registry and repositories the qa_agent_linux and qa_dca jobs publish the e2e images to
E2E_REGISTRY = "669783387624.dkr.ecr.us-east-1.amazonaws.com"
E2E_AGENT_REPOSITORY = "agent-qa"
E2E_CLUSTER_AGENT_REPOSITORY = "cluster-agent-qa"
ARCHITECTURES = ("amd64", "arm64")
# Length of CI_COMMIT_SHORT_SHA, used in the image tags
SHORT_SHA_LENGTH = 8

# Like SMP, candidates committed longer ago than this are not considered, but the first one
MAX_BASELINE_AGE = timedelta(days=4) - timedelta(hours=1)
# First-parent ancestors read from git: far more commits than the base branch gets in MAX_BASELINE_AGE
MAX_CANDIDATES = 1000

# Errors of `crane digest` meaning that the image does not exist, as opposed to e.g. an authentication failure
_MISSING_IMAGE_ERRORS = ("MANIFEST_UNKNOWN", "NAME_UNKNOWN", "404 Not Found")
# The cluster-agent amd64 build tags, from which the pipelines that built a commit are found
_CLUSTER_AGENT_BUILD_TAG_RE = re.compile(rf"^v(\d+)-([0-9a-f]{{{SHORT_SHA_LENGTH}}})-amd64$")


@dataclass(frozen=True)
class Commit:
    sha: str
    # Committer date, as a Unix timestamp
    timestamp: int

    @property
    def short_sha(self) -> str:
        return self.sha[:SHORT_SHA_LENGTH]


@dataclass(frozen=True)
class Baseline:
    """The images built for the commit `sha` by the pipeline `pipeline_id`."""

    sha: str
    pipeline_id: int

    @property
    def short_sha(self) -> str:
        return self.sha[:SHORT_SHA_LENGTH]

    @property
    def tag(self) -> str:
        return f"{self.pipeline_id}-{self.short_sha}"

    @property
    def agent_sources(self) -> list[str]:
        return [f"{SOURCE_AGENT_REPOSITORY}:v{self.tag}-7-{arch}" for arch in ARCHITECTURES]

    @property
    def cluster_agent_sources(self) -> list[str]:
        return [f"{SOURCE_CLUSTER_AGENT_REPOSITORY}:v{self.tag}-{arch}" for arch in ARCHITECTURES]

    @property
    def agent_destination(self) -> str:
        """The dd-pkg destination of the Agent image, the one qa_agent_linux published for the baseline pipeline."""
        return f"{E2E_AGENT_REPOSITORY}:{self.tag}-linux"

    @property
    def cluster_agent_destination(self) -> str:
        """The dd-pkg destination of the Cluster Agent image, the one qa_dca published for the baseline pipeline."""
        return f"{E2E_CLUSTER_AGENT_REPOSITORY}:{self.tag}"

    def dotenv_variables(self) -> dict[str, str]:
        """
        The variables of the dotenv report of the CI job resolving the baseline.

        The E2E_BENCHMARKEKS_BASELINE_* ones are read by the benchmarkeks suite, the others by the CI job publishing
        the baseline images.
        """
        return {
            "E2E_BENCHMARKEKS_BASELINE_SHA": self.sha,
            "E2E_BENCHMARKEKS_BASELINE_AGENT_IMAGE": f"{E2E_REGISTRY}/{self.agent_destination}",
            "E2E_BENCHMARKEKS_BASELINE_CLUSTER_AGENT_IMAGE": f"{E2E_REGISTRY}/{self.cluster_agent_destination}",
            "BENCHMARKEKS_BASELINE_AGENT_SOURCES": ",".join(self.agent_sources),
            "BENCHMARKEKS_BASELINE_CLUSTER_AGENT_SOURCES": ",".join(self.cluster_agent_sources),
            "BENCHMARKEKS_BASELINE_AGENT_DESTINATION": self.agent_destination,
            "BENCHMARKEKS_BASELINE_CLUSTER_AGENT_DESTINATION": self.cluster_agent_destination,
        }


class BaselineNotFoundError(Exception):
    """
    No candidate commit has images.

    `commit` is the candidate that was too old to be considered, None if there was no candidate left.
    """

    def __init__(self, commit: Commit | None):
        super().__init__(
            f"Commit {commit.sha} is too old to be a baseline"
            if commit
            else "No candidate commit left for the baseline"
        )
        self.commit = commit


def parse_rev_list(output: str) -> list[Commit]:
    """Parses the output of `git rev-list --timestamp`."""
    commits = []
    for line in output.splitlines():
        if line.strip():
            timestamp, sha = line.split()
            commits.append(Commit(sha, int(timestamp)))
    return commits


def index_pipeline_ids(cluster_agent_tags: Iterable[str]) -> dict[str, list[int]]:
    """Maps the short SHAs built by pipelines to the IDs of these pipelines, newest first."""
    pipeline_ids = defaultdict(list)
    for tag in cluster_agent_tags:
        match = _CLUSTER_AGENT_BUILD_TAG_RE.match(tag.strip())
        if match:
            pipeline_ids[match[2]].append(int(match[1]))
    return {short_sha: sorted(ids, reverse=True) for short_sha, ids in pipeline_ids.items()}


def is_missing_image_error(stderr: str) -> bool:
    """Tells whether the error output of a failed `crane digest` means that the image does not exist."""
    return any(error in stderr for error in _MISSING_IMAGE_ERRORS)


def find_baseline(
    candidates: Iterable[Commit],
    pipeline_ids: Mapping[str, Sequence[int]],
    images_exist: Callable[[Baseline], bool],
    oldest_timestamp: int,
) -> Baseline:
    """
    Returns the first baseline whose images exist, trying the candidates in order, and their pipelines newest first.

    Like SMP, the age of the first candidate is not checked, so that it can be the baseline however old it is, but the
    search stops at the first other candidate committed at or before `oldest_timestamp`: its images have likely expired.
    """
    for index, commit in enumerate(candidates):
        if index > 0 and commit.timestamp <= oldest_timestamp:
            raise BaselineNotFoundError(commit)
        for pipeline_id in pipeline_ids.get(commit.short_sha, ()):
            baseline = Baseline(commit.sha, pipeline_id)
            if images_exist(baseline):
                return baseline
    raise BaselineNotFoundError(None)


def format_dotenv(variables: Mapping[str, str]) -> str:
    """Formats variables, whose values must not need quoting, as a GitLab dotenv report or a shell script."""
    return "".join(f"{name}={value}\n" for name, value in variables.items())


def resolve_baseline(ctx, base_branch: str, branch: str, fetch: bool = True) -> Baseline:
    """
    Returns the baseline of the checked out commit of branch, or exits with an actionable message when there is none.

    The search starts from the merge base with base_branch, or from the parent commit on base_branch.
    """
    if branch == base_branch:
        start, start_description = "HEAD~1", "the parent commit"
    else:
        if fetch:
            refspec = f"+refs/heads/{base_branch}:refs/remotes/origin/{base_branch}"
            if ctx.run(join_command(["git", "fetch", "origin", refspec]), hide=True, warn=True).exited != 0:
                print(
                    color_message(
                        f"Warning: could not fetch {base_branch}, using the local origin/{base_branch}", Color.ORANGE
                    )
                )
        start = get_common_ancestor(ctx, "HEAD", f"origin/{base_branch}")
        start_description = f"the merge base with origin/{base_branch}"
    rev_list = ctx.run(
        join_command(["git", "rev-list", "--first-parent", "--timestamp", f"--max-count={MAX_CANDIDATES}", start]),
        hide=True,
    )
    candidates = parse_rev_list(rev_list.stdout)
    print(
        f"Branch {branch}, base branch {base_branch}: looking for the baseline from {start_description}, {candidates[0].sha}"
    )

    print(f"Listing the tags of {SOURCE_CLUSTER_AGENT_REPOSITORY}")
    tags = ctx.run(join_command(["crane", "ls", SOURCE_CLUSTER_AGENT_REPOSITORY]), hide=True).stdout.splitlines()
    pipeline_ids = index_pipeline_ids(tags)

    def commit_date(timestamp: int) -> str:
        return datetime.fromtimestamp(timestamp, UTC).isoformat()

    def checked(commits):
        for commit in commits:
            print(
                f"Commit {commit.sha} ({commit_date(commit.timestamp)}): pipelines {pipeline_ids.get(commit.short_sha, [])}"
            )
            yield commit

    def image_exists(ref: str) -> bool:
        result = ctx.run(join_command(["crane", "digest", ref]), hide=True, warn=True)
        if result.exited == 0:
            return True
        if is_missing_image_error(result.stderr):
            print(f"  {ref} not found")
            return False
        raise Exit(f"Could not check whether {ref} exists:\n{result.stderr}", code=1)

    cluster_agent_tags = set(tags)

    def images_exist(baseline) -> bool:
        # The tags of the Cluster Agent repository are already listed, unlike the ones of the Agent repository.
        return all(ref.rpartition(":")[2] in cluster_agent_tags for ref in baseline.cluster_agent_sources) and all(
            image_exists(ref) for ref in baseline.agent_sources
        )

    oldest_timestamp = int((datetime.now(UTC) - MAX_BASELINE_AGE).timestamp())
    try:
        baseline = find_baseline(checked(candidates), pipeline_ids, images_exist, oldest_timestamp)
    except BaselineNotFoundError as e:
        if e.commit is None:
            message = f"No images were found for the {len(candidates)} first-parent ancestors of {candidates[0].sha}."
        else:
            message = (
                f"The baseline commit ({e.commit.sha}, {commit_date(e.commit.timestamp)}) for this branch ({branch}) is "
                f"too old for benchmarkeks: no images were found for the more recent commits from {candidates[0].sha}. "
                f"Please update your branch by merging or rebasing it onto an up-to-date {base_branch} branch."
            )
        raise Exit(color_message(message, Color.RED), code=1) from e
    return baseline
