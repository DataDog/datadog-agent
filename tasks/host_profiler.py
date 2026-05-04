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
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_version_ldflags
from tasks.libs.releasing.json import get_current_milestone
from tasks.libs.releasing.version import query_version

EBPF_PROFILER_MODULE = "go.opentelemetry.io/ebpf-profiler"
CILIUM_EBPF_MODULE = "github.com/cilium/ebpf"
PPROFILE_MODULE = "go.opentelemetry.io/collector/pdata/pprofile"
PPROFILE_MAX_VERSION = "v0.150.0"

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

    branch = os.environ.get("CI_COMMIT_REF_SLUG")
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
    """Parse a release semver string (e.g. 'v0.150.0') into a tuple of ints.

    Only handles clean major.minor.patch tags — pseudo-versions or pre-release
    suffixes will raise Exit with a clear message.
    """
    v = version.lstrip("v")
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
