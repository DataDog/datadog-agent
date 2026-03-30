import json
import os
import shutil
import sys

from invoke import task
from invoke.context import Context
from invoke.exceptions import Exit

from tasks.build_tags import get_default_build_tags
from tasks.libs.common.color import color_message
from tasks.libs.common.constants import ALLOWED_REPO_NIGHTLY_BRANCHES
from tasks.libs.common.git import get_current_branch
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_version_ldflags
from tasks.libs.releasing.json import get_current_milestone
from tasks.libs.releasing.version import query_version

EBPF_PROFILER_MODULE = "go.opentelemetry.io/ebpf-profiler"
CILIUM_EBPF_MODULE = "github.com/cilium/ebpf"

BIN_NAME = "host-profiler"
BIN_DIR = os.path.join(".", "bin", "host-profiler")
BIN_PATH = os.path.join(BIN_DIR, bin_name("host-profiler"))


def _get_profiler_agent_version(ctx):
    """Return a profiler-specific AgentVersion string, or None to use the default.

    Two deployment contexts are handled:
      - Nightly (same condition as nightly relenv trigger):
          DDR_WORKFLOW_ID set + CI_COMMIT_BRANCH == "main" + BUCKET_BRANCH in nightly set
          → "7.79.0-nightly_git.101.89faa04"
      - Dev branch (BUCKET_BRANCH == "dev", covers both branch standalone and devtest):
          → "7.79.0-devel_git.101.89faa04.<branch_slug>"

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
        return f"{version}-nightly_git.{commits}.{git_sha}"

    branch = os.environ.get("CI_COMMIT_REF_SLUG") or get_current_branch(ctx)
    pre_label = pre if pre else "devel"
    return f"{version}-{pre_label}_git.{commits}.{git_sha}.{branch}"


@task
def build(ctx):
    """
    Build the host profiler
    """

    if os.path.exists(BIN_PATH):
        os.remove(BIN_PATH)

    env = {"GO111MODULE": "on"}
    build_tags = get_default_build_tags(build="host-profiler")
    ldflags = get_version_ldflags(ctx)
    if profiler_version := _get_profiler_agent_version(ctx):
        ldflags += f" -X {REPO_PATH}/pkg/version.AgentVersion={profiler_version}"
    if os.environ.get("DELVE"):
        gcflags = "all=-N -l"
    else:
        gcflags = ""

    # generate windows resources
    if sys.platform == 'win32':
        raise Exit("Windows is not supported for host-profiler")

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


@task
def update_golden_tests(ctx):
    """
    Update golden test files for host-profiler converters
    """
    print("Updating golden test files...")

    test_paths = ["comp/host-profiler/collector/impl/converters", "comp/host-profiler/collector/impl/agentprovider"]
    for path in test_paths:
        with ctx.cd(path):
            ctx.run("go test -tags test -update")

    print("Golden test files updated successfully!")


@task
def validate_deps(ctx: Context):
    """Check that the agent's cilium/ebpf version is compatible with the
    opentelemetry-ebpf-profiler.

    cilium/ebpf introduces breaking API changes across minor versions. Bumping
    it in the agent without first updating the profiler fork can silently break
    eBPF unwinding at runtime.

    TODO: This is a short-term guardrail. The long-term solution is to mirror
    the profiler's coredump test data to Datadog-owned blob storage and run
    the profiler's unwinding e2e tests directly from the agent CI whenever a
    common transitive dependency changes.
    """
    # Download the profiler module (extracts it from the cache if needed) and
    # get the path to its go.mod directly from the download output.
    res = ctx.run(f"go mod download -json {EBPF_PROFILER_MODULE}", hide=True, warn=True)
    if not res or not res.ok:
        raise Exit(f"Could not download {EBPF_PROFILER_MODULE}.", code=1)

    go_mod_path = json.loads(res.stdout).get("GoMod")
    if not go_mod_path:
        raise Exit(f"Could not locate go.mod for {EBPF_PROFILER_MODULE}.", code=1)

    # Parse the profiler's go.mod to find its required cilium/ebpf version.
    res = ctx.run(f"go mod edit -json {go_mod_path}", hide=True, warn=True)
    if not res or not res.ok:
        raise Exit(f"Could not parse {EBPF_PROFILER_MODULE} go.mod.", code=1)
    profiler_requires = {req["Path"]: req["Version"] for req in json.loads(res.stdout).get("Require", [])}

    profiler_version = profiler_requires.get(CILIUM_EBPF_MODULE)
    if profiler_version is None:
        raise Exit(f"{CILIUM_EBPF_MODULE} not found in {EBPF_PROFILER_MODULE} go.mod.", code=1)

    # Get the version the agent resolved.
    res = ctx.run(f"go list -m -json {CILIUM_EBPF_MODULE}", hide=True, warn=True)
    if not res or not res.ok:
        raise Exit(f"Could not resolve {CILIUM_EBPF_MODULE} in agent go.mod.", code=1)
    agent_version = json.loads(res.stdout).get("Version", "")

    # Patch-level differences are fine; major.minor must match.
    if agent_version.split(".")[:2] == profiler_version.split(".")[:2]:
        print(
            color_message(
                f"OK: {CILIUM_EBPF_MODULE} {agent_version} (agent) is compatible with {profiler_version} ({EBPF_PROFILER_MODULE})",
                "green",
            )
        )
    else:
        print(
            color_message(
                f"MISMATCH: {CILIUM_EBPF_MODULE} version is incompatible with {EBPF_PROFILER_MODULE}!\n"
                f"  Agent uses:     {agent_version}\n"
                f"  Profiler needs: {profiler_version}\n"
                f"  Please reach out to #profiling-full-host-project to update the profiler fork and validate its e2e tests before bumping cilium/ebpf here.",
                "red",
            )
        )
        raise Exit(code=1)
