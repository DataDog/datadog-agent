import json
import os
import shutil
import sys
import time

from invoke import task
from invoke.context import Context
from invoke.exceptions import Exit

from tasks.build_tags import get_default_build_tags
from tasks.libs.build.bazel import build_binary_with_bazel
from tasks.libs.common.color import color_message
from tasks.libs.common.constants import ALLOWED_REPO_NIGHTLY_BRANCHES
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags
from tasks.libs.releasing.json import get_current_milestone
from tasks.libs.releasing.version import query_version
from tasks.schema.generate import schema_codegen

EBPF_PROFILER_MODULE = "go.opentelemetry.io/ebpf-profiler"
CILIUM_EBPF_MODULE = "github.com/cilium/ebpf"
PPROFILE_MODULE = "go.opentelemetry.io/collector/pdata/pprofile"
PPROFILE_MAX_VERSION = "v0.165.1"

BIN_NAME = "host-profiler"
BIN_DIR = os.path.join(".", "bin", "host-profiler")
BIN_PATH = os.path.join(BIN_DIR, bin_name("host-profiler"))

# See .gitlab/deploy/internal_image_deploy/internal_image_deploy.yml.
INTERNAL_JOB = "publish_internal_container_image-host_profiler_standalone"
# See .gitlab/deploy/dev_container_deploy/docker_linux.yml.
DEV_BRANCH_JOB = "dev_branch-host-profiler-standalone"
DEVTEST_JOB = "dev_deploy-host-profiler-devtest"
DEVTEST_BRIDGE = "deploy_host_profiler_to_reliability_env_devtest"
FAILED_STATUSES = ("failed", "canceled", "skipped")


def _get_profiler_agent_version(ctx):
    """Return a profiler-specific AgentVersion string, or None to use the default.

    Two deployment contexts are handled:
      - Nightly (same condition as nightly relenv trigger):
          DDR_WORKFLOW_ID set + CI_COMMIT_BRANCH == "main" + BUCKET_BRANCH in nightly set
          → "7.79.0-nightly-git.101.89faa04"
      - Dev branch (BUCKET_BRANCH == "dev", covers both branch standalone and devtest):
          → "7.79.0-devel-git.101.89faa04.<branch_slug>"

    Returns None for stable/beta/release builds and local builds.
    """
    bucket_branch = os.environ.get("BUCKET_BRANCH", "")

    is_nightly = (
        bool(os.environ.get("DDR_WORKFLOW_ID"))
        and os.environ.get("CI_COMMIT_BRANCH") == "main"
        and bucket_branch in ALLOWED_REPO_NIGHTLY_BRANCHES
    )
    is_dev = bucket_branch == "dev"

    if not is_nightly and not is_dev:
        return None

    major_version = get_current_milestone().split('.')[0]
    version, pre, commits, git_sha, _ = query_version(ctx, major_version=major_version)

    if is_nightly:
        return f"{version}-nightly-git.{commits}.{git_sha}"

    branch = os.environ.get("CI_COMMIT_REF_SLUG")
    pre_label = pre if pre else "devel"
    return f"{version}-{pre_label}-git.{commits}.{git_sha}.{branch}"


@task
def build(ctx, enable_bazel=False):
    """
    Build the host profiler
    """

    if os.path.exists(BIN_PATH):
        os.remove(BIN_PATH)

    # generate windows resources
    if sys.platform == 'win32':
        raise Exit("Windows is not supported for host-profiler")

    if enable_bazel:
        args = []
        if profiler_version := _get_profiler_agent_version(ctx):
            args.append(f"--repo_env=FORCE_AGENT_VERSION={profiler_version}")
        build_binary_with_bazel("//cmd/host-profiler:host-profiler", args=args, bin_path=BIN_PATH)
    else:
        build_tags = get_default_build_tags(build="host-profiler")
        ldflags, gcflags, env = get_build_flags(ctx)
        if not os.environ.get("DELVE"):
            ldflags += " -w"
        if profiler_version := _get_profiler_agent_version(ctx):
            ldflags += f" -X {REPO_PATH}/pkg/version.AgentVersion={profiler_version}"

        go_build(
            ctx,
            f"{REPO_PATH}/cmd/host-profiler",
            mod="readonly",
            build_tags=build_tags,
            ldflags=ldflags,
            gcflags=gcflags,
            bin_path=BIN_PATH,
            env=env,
        )

    dist_folder = os.path.join(BIN_DIR, "dist")
    if os.path.exists(dist_folder):
        shutil.rmtree(dist_folder)
    os.mkdir(dist_folder)

    shutil.copy(
        "./cmd/host-profiler/dist/host-profiler-config.yaml",
        os.path.join(dist_folder, "host-profiler-config.yaml"),
    )


def _latest(items, name):
    """Return the item named `name` with the highest id, i.e. the last retry."""
    return max((i for i in items if i.name == name), key=lambda i: i.id, default=None)


def _report(seen: dict, name: str, status: str, url: str = ""):
    if seen.get(name) != status:
        seen[name] = status
        print(f"{time.strftime('%H:%M:%S')} {name}: {status} {url}".rstrip(), flush=True)


def _ref_slug(ref: str) -> str:
    """GitLab's CI_COMMIT_REF_SLUG."""
    import re

    return re.sub(r"[^a-z0-9]", "-", ref.lower())[:63].strip("-")


def _follow(repo, pipeline, names: list[str], timeout: int):
    """Wait for the jobs in `names` and, for devtest, the reliability env trigger. Print published images."""
    import re

    all_jobs = pipeline.jobs.list(all=True)
    job_ids = {}
    for name in names:
        job = _latest(all_jobs, name)
        if job is None:
            raise Exit(f"{name} is not part of the pipeline.", code=1)
        job_ids[name] = job.id
    pending = set(names) | ({DEVTEST_BRIDGE} if DEVTEST_JOB in names else set())
    tag = child_url = None
    seen: dict[str, str] = {}
    deadline = time.time() + timeout

    while time.time() < deadline:
        for name in [n for n in names if n in pending]:
            job = repo.jobs.get(job_ids[name])
            if job.status in FAILED_STATUSES:
                # Jobs may retry themselves on failure.
                job_ids[name] = _latest(pipeline.jobs.list(all=True), name).id
                job = repo.jobs.get(job_ids[name])
            _report(seen, name, job.status, job.web_url)
            if name == INTERNAL_JOB and child_url is None and job.status in ("running", "success"):
                log = job.trace().decode(errors="replace")
                if tag is None and (m := re.search(r"'RELEASE_TAG': '([^']+)'", log)):
                    tag = m.group(1)
                    print(f"Tag: {tag}", flush=True)
                if m := re.search(r"Created a child pipeline with id=\d+, url=(\S+)", log):
                    child_url = m.group(1)
                    print(f"ddoghq/images pipeline: {child_url}", flush=True)
            if job.status == "manual":
                raise Exit(f"{name} did not start automatically: does {pipeline.ref} include its CI rule?", code=1)
            if job.status in FAILED_STATUSES:
                raise Exit(f"{name} {job.status}: {job.web_url}", code=1)
            if job.status == "success":
                pending.discard(name)
                if name == INTERNAL_JOB:
                    print(color_message(f"Published registry.ddbuild.io/ddot-ebpf:{tag}", "green"), flush=True)
                elif name == DEV_BRANCH_JOB:
                    image = f"docker.io/datadog/ddot-ebpf-dev:{_ref_slug(pipeline.ref)}"
                    print(color_message(f"Published {image}", "green"), flush=True)

        if DEVTEST_BRIDGE in pending:
            bridge = _latest(pipeline.bridges.list(all=True), DEVTEST_BRIDGE)
            if bridge is None:
                raise Exit(f"{DEVTEST_BRIDGE} is not part of the pipeline.", code=1)
            _report(seen, DEVTEST_BRIDGE, bridge.status, (bridge.downstream_pipeline or {}).get("web_url", ""))
            if bridge.status in FAILED_STATUSES:
                raise Exit(f"{DEVTEST_BRIDGE} {bridge.status}: {pipeline.web_url}", code=1)
            if bridge.status == "success":
                pending.discard(DEVTEST_BRIDGE)
                print(color_message("Deployed to the reliability devtest env.", "green"), flush=True)

        if not pending:
            return
        time.sleep(30)
    raise Exit("Timed out.", code=1)


@task
def publish(
    ctx,
    internal=True,
    dev_branch=False,
    devtest=False,
    git_ref="",
    pipeline_id=None,
    follow=True,
    timeout=3 * 3600,
):
    """
    Publish the host profiler image of a pushed branch, without needing a PR.

    --internal (default) publishes registry.ddbuild.io/ddot-ebpf:dev-<branch>-<sha> through ddoghq/images.
    --dev-branch publishes the public docker.io/datadog/ddot-ebpf-dev:<branch slug>, skipping ddoghq/images.
    --devtest overwrites the shared ddot-ebpf-dev:devtest-latest (used by relenv candidate-experimental)
    and deploys it to the reliability devtest env.
    --pipeline-id plays the jobs of an existing pipeline instead, which requires their dependencies to be built.
    """
    from gitlab.exceptions import GitlabGetError, GitlabJobPlayError

    from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
    from tasks.libs.common.git import get_commit_sha, get_current_branch
    from tasks.libs.pipeline.tools import (
        FilteredOutException,
        cancel_pipelines_with_confirmation,
        get_running_pipelines_on_same_ref,
        trigger_agent_pipeline,
    )

    targets = {
        INTERNAL_JOB: ("PUBLISH_HOST_PROFILER_INTERNAL", internal),
        DEV_BRANCH_JOB: ("PUBLISH_HOST_PROFILER_DEV_BRANCH", dev_branch),
        DEVTEST_JOB: ("DEPLOY_HOST_PROFILER_DEVTEST", devtest),
    }
    jobs = {name: var for name, (var, enabled) in targets.items() if enabled}
    if not jobs:
        raise Exit("Nothing to publish: --no-internal requires --dev-branch or --devtest.", code=1)

    repo = get_gitlab_repo()

    if pipeline_id:
        if git_ref:
            raise Exit("--git-ref and --pipeline-id are mutually exclusive.", code=1)
        pipeline = repo.pipelines.get(pipeline_id)
        manual = {j.name: j for j in pipeline.jobs.list(scope="manual", all=True)}
        for name in jobs:
            if name not in manual:
                raise Exit(f"{name} is not playable: dependencies not built yet, or already played.", code=1)
        for name in jobs:
            try:
                repo.jobs.get(manual[name].id, lazy=True).play()
            except GitlabJobPlayError as e:
                raise Exit(f"Could not play {name}: {e}", code=1) from e
    else:
        ref = git_ref or get_current_branch(ctx)
        try:
            remote_sha = repo.branches.get(ref).commit["id"]
        except GitlabGetError as e:
            raise Exit(f"Branch {ref} is not pushed.", code=1) from e
        if not git_ref and remote_sha != get_commit_sha(ctx):
            print(color_message(f"WARNING: local HEAD differs from pushed {ref} ({remote_sha[:11]}).", "orange"))

        running = get_running_pipelines_on_same_ref(repo, ref)
        if running:
            print(f"There are already {len(running)} pipeline(s) running on {ref}.")
            cancel_pipelines_with_confirmation(repo, running)

        try:
            pipeline = trigger_agent_pipeline(repo, ref, "dev", variables={var: "true" for var in jobs.values()})
        except FilteredOutException as e:
            raise Exit(f"The pipeline for {ref} does not match any workflow rule.", code=1) from e

    print(color_message("Pipeline: ", "blue") + color_message(pipeline.web_url, "green"), flush=True)
    if follow:
        _follow(repo, pipeline, list(jobs), timeout)


@task
def update_golden_tests(ctx):
    """
    Update golden test files for host-profiler converters
    """
    print("Updating golden test files...")

    # TODO: remove once Bazel is used to build the Agent
    schema_codegen(ctx)

    test_paths = ["comp/host-profiler/collector/impl/converters", "comp/host-profiler/collector/impl/agentprovider"]
    for path in test_paths:
        with ctx.cd(path):
            ctx.run("go test -tags test -update")

    print("Golden test files updated successfully!")


def _get_profiler_requires(ctx: Context) -> dict[str, str]:
    """Download the ebpf-profiler fork and return its go.mod Require entries as {path: version}."""
    res = ctx.run(f"go mod download -json {EBPF_PROFILER_MODULE}", hide=True, warn=True)
    if not res or not res.ok:
        raise Exit(f"Could not download {EBPF_PROFILER_MODULE}.", code=1)
    go_mod_path = json.loads(res.stdout).get("GoMod")
    if not go_mod_path:
        raise Exit(f"Could not locate go.mod for {EBPF_PROFILER_MODULE}.", code=1)
    res = ctx.run(f"go mod edit -json {go_mod_path}", hide=True, warn=True)
    if not res or not res.ok:
        raise Exit(f"Could not parse {EBPF_PROFILER_MODULE} go.mod.", code=1)
    return {req["Path"]: req["Version"] for req in json.loads(res.stdout).get("Require", [])}


def _get_agent_module_version(ctx: Context, module: str) -> str:
    """Return the version of a module as resolved by the agent's go.mod."""
    res = ctx.run(f"go list -m -json {module}", hide=True, warn=True)
    if not res or not res.ok:
        raise Exit(f"Could not resolve {module} in agent go.mod.", code=1)
    version = json.loads(res.stdout).get("Version")
    if not version:
        raise Exit(f"No version found for {module} in agent go.mod.", code=1)
    return version


def _parse_semver(version: str) -> tuple[int, ...]:
    """Parse a semver string (e.g. 'v0.150.0' or pseudo-versions) into a tuple of ints.

    Strips any pre-release suffix (everything from the first '-' onward) before
    parsing, so pseudo-versions like 'v0.155.1-0.20260625...' are handled correctly.
    """
    v = version.lstrip("v").split("-")[0]
    parts = v.split(".")
    try:
        return tuple(int(p) for p in parts)
    except ValueError as e:
        raise Exit(f"Cannot parse version {version!r} as semver (expected vX.Y.Z).", code=1) from e


@task
def validate_deps(ctx: Context):
    """Check that shared transitive dependencies between the agent and the
    opentelemetry-ebpf-profiler remain compatible.

    Currently validates:
      - cilium/ebpf: major.minor must match between agent and profiler fork.
      - pdata/pprofile: must be at most PPROFILE_MAX_VERSION or match the
        profiler fork's version.

    TODO: This is a short-term guardrail. The long-term solution is to mirror
    the profiler's coredump test data to Datadog-owned blob storage and run
    the profiler's unwinding e2e tests directly from the agent CI whenever a
    common transitive dependency changes.
    """
    profiler_requires = _get_profiler_requires(ctx)
    ok = True

    # --- cilium/ebpf: major.minor must match ---
    profiler_cilium = profiler_requires.get(CILIUM_EBPF_MODULE)
    if profiler_cilium is None:
        print(color_message(f"{CILIUM_EBPF_MODULE} not found in {EBPF_PROFILER_MODULE} go.mod.", "red"))
        ok = False
    else:
        agent_cilium = _get_agent_module_version(ctx, CILIUM_EBPF_MODULE)
        if agent_cilium.split(".")[:2] == profiler_cilium.split(".")[:2]:
            print(
                color_message(
                    f"OK: {CILIUM_EBPF_MODULE} {agent_cilium} (agent) is compatible with {profiler_cilium} ({EBPF_PROFILER_MODULE})",
                    "green",
                )
            )
        else:
            print(
                color_message(
                    f"MISMATCH: {CILIUM_EBPF_MODULE} version is incompatible with {EBPF_PROFILER_MODULE}!\n"
                    f"  Agent uses:     {agent_cilium}\n"
                    f"  Profiler needs: {profiler_cilium}\n"
                    f"  Please reach out to #profiling-full-host-project to update the profiler fork and validate its e2e tests before bumping cilium/ebpf here.",
                    "red",
                )
            )
            ok = False

    # --- pdata/pprofile: at most PPROFILE_MAX_VERSION or matching profiler fork ---
    # pprofile is experimental upstream and has constant breaking changes,
    # so we cap the version to avoid silent incompatibilities with the profiler fork.
    agent_pprofile = _get_agent_module_version(ctx, PPROFILE_MODULE)
    profiler_pprofile = profiler_requires.get(PPROFILE_MODULE)

    if profiler_pprofile and agent_pprofile == profiler_pprofile:
        print(
            color_message(
                f"OK: {PPROFILE_MODULE} {agent_pprofile} (agent) matches {profiler_pprofile} ({EBPF_PROFILER_MODULE})",
                "green",
            )
        )
    elif _parse_semver(agent_pprofile) <= _parse_semver(PPROFILE_MAX_VERSION):
        print(
            color_message(
                f"OK: {PPROFILE_MODULE} {agent_pprofile} (agent) is within allowed ceiling {PPROFILE_MAX_VERSION}",
                "green",
            )
        )
    else:
        profiler_hint = (
            f" or update the profiler fork to support it (currently requires {profiler_pprofile})"
            if profiler_pprofile
            else ""
        )
        print(
            color_message(
                f"MISMATCH: {PPROFILE_MODULE} version exceeds allowed bounds!\n"
                f"  Agent uses:      {agent_pprofile}\n"
                f"  Max allowed:     {PPROFILE_MAX_VERSION}\n"
                f"  Profiler uses:   {profiler_pprofile or 'not found'}\n"
                f"  Please either pin {PPROFILE_MODULE} to at most {PPROFILE_MAX_VERSION}{profiler_hint}.\n"
                f"  Reach out to #profiling-full-host-project for guidance.",
                "red",
            )
        )
        ok = False

    if not ok:
        raise Exit(code=1)
