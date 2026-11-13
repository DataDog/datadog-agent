"""
Baseline resolution of the benchmarkeks e2e suite (TestBenchmarkEKSSuite in test/new-e2e/tests/containers).

The suite compares the resource usage of the Agent of the pipeline under test with the one of a baseline Agent. The
baseline commit is chosen like the SMP regression detector does (.gitlab/test/functional_test/regression_detector.yml):
the merge base with the base branch, or the parent commit on the base branch, walking first parents back to the first
commit with images.

The images of the agent-qa ECR repositories e2e tests pull from only live for a day, so the baseline images are looked
up in the registry the container build jobs push to, which keeps them for months, to be published to agent-qa again.

This module also writes the pull request comment of the benchmark, which its CI jobs update as it progresses: how to
start it, that it runs, then its results.
"""

from __future__ import annotations

import json
import os
import re
from collections import defaultdict
from collections.abc import Callable, Iterable, Mapping, Sequence
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from urllib.parse import quote

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.utils import join_command

# Repositories the container build jobs push the images of every pipeline to, tagged `v<pipeline ID>-<short SHA>-...`:
# SRC_AGENT and SRC_DCA of .docker_variables in CI
SOURCE_AGENT_REPOSITORY = os.environ.get("SRC_AGENT", "registry.ddbuild.io/ci/datadog-agent/agent")
SOURCE_CLUSTER_AGENT_REPOSITORY = os.environ.get("SRC_DCA", "registry.ddbuild.io/ci/datadog-agent/cluster-agent")
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

# The CI job starting the benchmark, manual unless the push changes the benchmark itself: the other jobs of the
# benchmark need it
BASELINE_JOB_NAME = "new-e2e-containers-benchmarkeks-baseline"
# The CI job running the benchmark, whose log ends with its report
BENCHMARK_JOB_NAME = "new-e2e-containers-benchmarkeks"
# The title of the pull request comment, which identifies it: the CI jobs of the benchmark update the same comment
COMMENT_TITLE = "Agent performance benchmark (benchmarkeks)"
# The title of the comment of the results of the benchmark before the comment above followed it, deleted when it is
# updated. TODO: remove once the pull requests whose benchmark ran before are closed.
LEGACY_COMMENT_TITLE = "Agent performance comparison (benchmarkeks)"
# The states of the benchmark the comment shows: to start, running, done
COMMENT_STATES = ("pending", "running", "done")
# The reports the benchmark writes, JSONReportFile and MarkdownReportFile of test/new-e2e/tests/containers/benchmark
JSON_REPORT_FILE = "report.json"
MARKDOWN_REPORT_FILE = "report.md"


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


class BaselineLookupError(Exception):
    """The baseline could not be resolved, for the reason the message gives to the user."""


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
    """
    Maps the short SHAs built by pipelines to the IDs of these pipelines, oldest first: the first pipeline of a commit
    builds it normally, where a later one may be a coverage pipeline, building instrumented binaries.
    """
    pipeline_ids = defaultdict(list)
    for tag in cluster_agent_tags:
        match = _CLUSTER_AGENT_BUILD_TAG_RE.match(tag)
        if match:
            pipeline_ids[match[2]].append(int(match[1]))
    return {short_sha: sorted(ids) for short_sha, ids in pipeline_ids.items()}


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
    Returns the first baseline whose images exist, trying the candidates in order, and their pipelines in the order
    of pipeline_ids.

    Like SMP, the age of the first candidate is not checked, so that it can be the baseline however old it is, but the
    search stops at the first other candidate committed at or before `oldest_timestamp`: a baseline that much older than
    the branch would also measure all the changes of the base branch in between.
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


def _run_checked(ctx, args: list[str], action: str) -> str:
    """Runs a command and returns its output, or raises BaselineLookupError if it fails, saying it could not `action`."""
    result = ctx.run(join_command(args), hide=True, warn=True)
    if result.exited != 0:
        raise BaselineLookupError(f"Could not {action}:\n{result.stderr}")
    return result.stdout


def resolve_baseline(ctx, base_branch: str, branch: str, fetch: bool = True) -> Baseline:
    """
    Returns the baseline of the checked out commit of branch, or raises BaselineLookupError with an actionable message
    when there is none.

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
        start_description = f"the merge base with origin/{base_branch}"
        start = _run_checked(
            ctx, ["git", "merge-base", "HEAD", f"origin/{base_branch}"], f"compute {start_description}"
        ).strip()
    rev_list = _run_checked(
        ctx,
        ["git", "rev-list", "--first-parent", "--timestamp", f"--max-count={MAX_CANDIDATES}", start],
        f"list the first-parent ancestors of {start_description} ({start})",
    )
    candidates = parse_rev_list(rev_list)
    if not candidates:
        raise BaselineLookupError(f"No commit found from {start_description} ({start})")
    print(
        f"Branch {branch}, base branch {base_branch}: looking for the baseline from {start_description}, {candidates[0].sha}"
    )

    print(f"Listing the tags of {SOURCE_CLUSTER_AGENT_REPOSITORY}")
    cluster_agent_tags = set(
        _run_checked(
            ctx, ["crane", "ls", SOURCE_CLUSTER_AGENT_REPOSITORY], f"list the tags of {SOURCE_CLUSTER_AGENT_REPOSITORY}"
        ).split()
    )
    pipeline_ids = index_pipeline_ids(cluster_agent_tags)

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
        raise BaselineLookupError(f"Could not check whether {ref} exists:\n{result.stderr}")

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
        raise BaselineLookupError(message) from e
    return baseline


@dataclass(frozen=True)
class Button:
    """
    A link of the pull request comment shown as a large badge of shields.io, to stand out like a button. Its text starts
    with an emoji telling the state of the benchmark at a glance.
    """

    text: str
    # The background color, as hex RGB
    color: str

    def badge_url(self) -> str:
        # In the path of a static badge, a dash separates the text from the color: the text doubles its own dashes and
        # underscores
        text = self.text.replace("-", "--").replace("_", "__")
        return f"https://img.shields.io/badge/{quote(text, safe='')}-{self.color}?style=for-the-badge"

    def markdown(self, url: str) -> str:
        return f"[![{self.text}]({self.badge_url()})]({url})"


# The benchmark is to be started, in the purple of Datadog
RUN_BUTTON = Button("\N{BLACK RIGHT-POINTING TRIANGLE} Run the benchmark", "632CA6")
# The benchmark runs: an hourglass whose sand is still flowing, in yellow
RUNNING_BUTTON = Button("\N{HOURGLASS WITH FLOWING SAND} Benchmark running", "DFB317")
# The benchmark completed: no metric regressed, whether some improved or not
PASSED_BUTTON = Button("\N{WHITE HEAVY CHECK MARK} No regression", "2EA44F")
# The benchmark completed, and a metric regressed
REGRESSION_BUTTON = Button("\N{CROSS MARK} Regression", "E05D44")
# The benchmark completed, but some metrics couldn't be compared
INCOMPLETE_BUTTON = Button("\N{WARNING SIGN}\N{VARIATION SELECTOR-16} Incomplete comparison", "FE7D37")
# The benchmark failed before comparing the versions
FAILED_BUTTON = Button("\N{WARNING SIGN}\N{VARIATION SELECTOR-16} Benchmark failed", "FE7D37")


def pending_comment(baseline_job_url: str | None, pipeline_url: str) -> str:
    """
    Returns the pull request comment explaining how to start the benchmark: a button linking to the manual job
    BASELINE_JOB_NAME, whose page has a button running it, or a link to the pipeline if the job's URL is unknown.
    """
    if baseline_job_url is None:
        action, button = f"play the manual job `{BASELINE_JOB_NAME}` of [the pipeline]({pipeline_url})", ""
    else:
        action = f"open the manual job `{BASELINE_JOB_NAME}` with the button below, then click **Run job**"
        button = f"\n{RUN_BUTTON.markdown(baseline_job_url)}\n"
    return (
        "This pull request changes code that the `benchmarkeks` performance benchmark measures. To compare the "
        f"resource usage of its Agent with the one of its base branch, {action}: the other jobs of the benchmark run "
        "after it, on an EKS cluster, for about 2 hours, and this comment then shows their progress and results.\n"
        f"{button}"
    )


def running_comment(benchmark_job_url: str, pipeline_url: str, commit: str, started_at: datetime) -> str:
    """
    Returns the pull request comment telling that the benchmark of commit started at started_at, in the pipeline at
    pipeline_url, with a button linking to benchmark_job_url.
    """
    return (
        f"{RUNNING_BUTTON.markdown(benchmark_job_url)}\n\n"
        f"The benchmark of commit `{commit}` started at {started_at.astimezone(UTC):%Y-%m-%d %H:%M} UTC in "
        f"[its pipeline]({pipeline_url}). It lasts about 2 hours, after which this comment shows its results.\n"
    )


def outcome_button(report: Mapping) -> Button:
    """
    Returns the button telling the outcome of the benchmark from its JSON report.
    """
    # The Go report marshals no results as null
    verdicts = {result.get("verdict") for result in report.get("results") or []}
    if "regression" in verdicts:
        return REGRESSION_BUTTON
    if "error" in verdicts:
        return INCOMPLETE_BUTTON
    return PASSED_BUTTON


def done_comment(
    benchmark_job_url: str, pipeline_url: str, commit: str, report: Mapping | None, report_markdown: str | None
) -> str:
    """
    Returns the pull request comment of the results of the benchmark of commit: a button telling its outcome, linking
    to benchmark_job_url, then its Markdown report. report and report_markdown are the JSON and Markdown reports (see
    read_reports), None if the benchmark failed before writing them.
    """
    if report is None or not report_markdown:
        return (
            f"{FAILED_BUTTON.markdown(benchmark_job_url)}\n\nThe benchmark of commit `{commit}` failed before comparing "
            f"the versions: see the log of [its job]({benchmark_job_url}).\n"
        )
    return (
        f"{outcome_button(report).markdown(benchmark_job_url)}\n\nResults of the benchmark of commit `{commit}`, in "
        f"[its pipeline]({pipeline_url}).\n\n{report_markdown}"
    )


def read_reports(report_dir: str) -> tuple[dict | None, str | None]:
    """
    Returns the JSON and Markdown reports of the benchmark in report_dir, each None if the benchmark didn't write it,
    or left it unreadable, e.g. truncated in the middle of the JSON or of a character.
    """
    if not report_dir:
        return None, None

    def read(name: str, load: Callable):
        # The reports are UTF-8, with emojis, whatever the locale of the job. OSError covers a missing or unreadable
        # file, ValueError the errors of JSON and of UTF-8 decoding.
        try:
            with open(os.path.join(report_dir, name), encoding="utf-8") as f:
                return load(f)
        except (OSError, ValueError):
            return None

    return read(JSON_REPORT_FILE, json.load), read(MARKDOWN_REPORT_FILE, lambda f: f.read())
