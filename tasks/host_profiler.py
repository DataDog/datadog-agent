import json
import os
import platform
import re
import shutil
import subprocess
import sys
import time
from concurrent.futures import ThreadPoolExecutor

from invoke import task
from invoke.context import Context
from invoke.exceptions import Exit

from tasks.build_tags import get_default_build_tags
from tasks.libs.build.bazel import build_binary_with_bazel
from tasks.libs.common.color import color_message
from tasks.libs.common.constants import ALLOWED_REPO_NIGHTLY_BRANCHES
from tasks.libs.common.git import get_current_branch
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags
from tasks.libs.releasing.json import get_current_milestone
from tasks.libs.releasing.version import query_version
from tasks.schema.generate import schema_codegen

EBPF_PROFILER_MODULE = "go.opentelemetry.io/ebpf-profiler"
CILIUM_EBPF_MODULE = "github.com/cilium/ebpf"
PPROFILE_MODULE = "go.opentelemetry.io/collector/pdata/pprofile"
PPROFILE_MAX_VERSION = "v0.164.1"

BIN_NAME = "host-profiler"
BIN_DIR = os.path.join(".", "bin", "host-profiler")
BIN_PATH = os.path.join(BIN_DIR, bin_name("host-profiler"))

LOCAL_IMAGE_REGISTRY = "registry.ddbuild.io"
LOCAL_IMAGE_REPOSITORY = "ddot-ebpf"
LOCAL_DEV_REGISTRY = "docker.io/datadog"
LOCAL_DEV_REPOSITORY = "ddot-ebpf-dev"
LOCAL_IMAGE_BUILD_CONTEXT = os.path.join(".", "Dockerfiles", "ddot-ebpf")

# Delay before the second build starts, so the first finishes rewriting the
# generated files (both runs execute schema_codegen).
CODEGEN_STAGGER_SECONDS = 5


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
def build(ctx, enable_bazel=False, bin_path=None):
    """
    Build the host profiler
    """
    bin_path = bin_path or BIN_PATH
    os.makedirs(os.path.dirname(os.path.abspath(bin_path)), exist_ok=True)
    if os.path.exists(bin_path):
        os.remove(bin_path)

    # generate windows resources
    if sys.platform == 'win32':
        raise Exit("Windows is not supported for host-profiler")

    if enable_bazel:
        args = []
        if profiler_version := _get_profiler_agent_version(ctx):
            args.append(f"--repo_env=FORCE_AGENT_VERSION={profiler_version}")
        build_binary_with_bazel("//cmd/host-profiler:host-profiler", args=args, bin_path=bin_path)
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
            bin_path=bin_path,
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


def _latest_base_image_tag(ctx: Context, registry: str) -> str:
    """Return the newest agent-base-image tag with both arches, without the arch suffix."""
    repo = f"{registry}/ci/datadog-agent/agent-base-image"
    res = ctx.run(f"crane ls {repo}", hide=True, echo=False, warn=True)
    if not res or not res.ok:
        raise Exit(f"Could not list tags of {repo} with crane, is it installed and authenticated?", code=1)
    pairs: dict[tuple[int, str], set[str]] = {}
    for tag_ in res.stdout.split():
        m = re.match(r"^v(\d+)-(\w+)-(amd64|arm64)$", tag_)
        if m:
            pairs.setdefault((int(m.group(1)), m.group(2)), set()).add(m.group(3))
    for (pipeline_id, sha), arches in sorted(pairs.items(), reverse=True):
        if arches == {"amd64", "arm64"}:
            return f"v{pipeline_id}-{sha}"
    raise Exit(f"No complete amd64/arm64 agent-base-image tag pair found in {repo}", code=1)


# host arch (uname -m) -> GOARCH
_HOST_GOARCH = {"x86_64": "amd64", "aarch64": "arm64"}
# GOARCH -> cross-compiler get_build_flags requires when cross-compiling
_CROSS_COMPILERS = {"arm64": "aarch64-linux-gnu-gcc", "amd64": "x86_64-linux-gnu-gcc"}


def _check_cross_compiler(arches: list[str]) -> None:
    """Fail with the install command if a target arch needs a missing cross toolchain."""
    host_arch = _HOST_GOARCH.get(platform.machine())
    for arch in arches:
        if arch == host_arch:
            continue
        compiler = _CROSS_COMPILERS[arch]
        if not shutil.which(compiler):
            package = {
                "arm64": "gcc-aarch64-linux-gnu g++-aarch64-linux-gnu",
                "amd64": "gcc-x86-64-linux-gnu g++-x86-64-linux-gnu",
            }[arch]
            raise Exit(
                f"Building for {arch} on a {host_arch} machine cross-compiles with GOARCH and needs the {arch} toolchain:\n"
                f"  sudo apt install {package}",
                code=1,
            )


def _run_parallel(jobs: list[tuple[str, list[str], dict[str, str] | None, int]], tail: int = 15) -> None:
    """Run (name, command, extra env, delay seconds) jobs as parallel subprocesses.

    Print the last `tail` lines of each output; fail on the first error.
    """

    def run_job(job):
        name, cmd, env, delay = job
        time.sleep(delay)
        return name, subprocess.run(cmd, env={**os.environ, **(env or {})}, capture_output=True, text=True)

    with ThreadPoolExecutor(max_workers=len(jobs)) as executor:
        futures = [executor.submit(run_job, job) for job in jobs]
        for future in futures:
            name, result = future.result()
            output = (result.stdout or "") + (result.stderr or "")
            ok = result.returncode == 0
            print(color_message(f"=== {name}: {'OK' if ok else 'FAILED'}", "green" if ok else "red"))
            if not ok:
                print(output)
                raise Exit(f"{name} failed with exit code {result.returncode}", code=1)
            if output.strip():
                print("\n".join(output.strip().splitlines()[-tail:]))


@task
def publish(
    ctx,
    tag=None,
    base_image_tag=None,
    registry=LOCAL_IMAGE_REGISTRY,
    repository=LOCAL_IMAGE_REPOSITORY,
    dev_registry=LOCAL_DEV_REGISTRY,
    dev_repository=LOCAL_DEV_REPOSITORY,
    publish_dev=True,
):
    """
    Build and push the ddot-ebpf (host profiler standalone) image locally, without
    CI (PROF-16525). Afterwards, a KDAO change only needs to reference
    registry.ddbuild.io/ddot-ebpf:<tag>.

    Build host-profiler for both arches in parallel (the non-host one
    cross-compiles with GOARCH), assemble the image like the CI
    docker_build_host_profiler_standalone_* jobs, and push multi-arch manifests to:

    - <registry>/<repository>:<tag> — default registry.ddbuild.io/ddot-ebpf:dev-<branch-slug>
    - <dev_registry>/<dev_repository>:<tag> — public dev registry, like the CI
      dev_branch-host-profiler-standalone job; skip with --no-publish-dev

    Requires go, jq, docker (ddtool credential helper for registry.ddbuild.io,
    plus a docker login with push access to the dev registry), crane, and the
    non-host arch toolchain (the preflight error prints the install command).

    Examples:
        dda inv host-profiler.publish
        dda inv host-profiler.publish --tag dev-my-experiment --base-image-tag v12345-abcdef
        dda inv host-profiler.publish --no-publish-dev  # internal registry only
    """
    if sys.platform != "linux":
        raise Exit("host-profiler.publish is only supported on Linux (build machine requirement)", code=1)

    arches = ["amd64", "arm64"]
    image = f"{registry}/{repository}"
    dev_image = f"{dev_registry}/{dev_repository}" if publish_dev else None
    if not tag:
        branch = get_current_branch(ctx)
        if not branch or branch == "HEAD":
            raise Exit("Could not determine the current branch (detached HEAD?), pass --tag explicitly", code=1)
        # GitLab-style slug, trimmed so dev-<slug>-<arch> stays under the 63-char tag limit
        slug = re.sub(r"[^a-zA-Z0-9_]+", "-", branch).strip("-").lower()[:50]
        if not slug:
            raise Exit(
                f"Branch name {branch!r} has no usable characters for an image tag, pass --tag explicitly", code=1
            )
        tag = f"dev-{slug}"

    _check_cross_compiler(arches)
    # Each arch builds straight to its own binary (bin/host-profiler/host-profiler.<arch>),
    # so the parallel builds never share an output path. The second build is staggered
    # because both run schema_codegen (inside go_build), which rewrites the same
    # generated files non-atomically. The content is identical, so without the
    # stagger the worst case is a failed build, not a broken image.
    jobs = [
        (
            f"host-profiler build {arch}",
            [sys.executable, "-m", "invoke", "host-profiler.build", f"--bin-path={BIN_PATH}.{arch}"],
            {"GOARCH": arch},
            CODEGEN_STAGGER_SECONDS * i,
        )
        for i, arch in enumerate(arches)
    ]
    for _, _, env, _ in jobs:
        print(color_message(f"=== Building host-profiler for {env['GOARCH']}", "green"))
    _run_parallel(jobs)

    # tools/host-profiler/prepare-image-context.sh is the single source of
    # truth for the image contents, shared with the CI jobs.
    context_files = ["host-profiler", "host-profiler-config.yaml", "seccomp-profile.json", "logging-seccomp.json"]
    try:
        base = base_image_tag or _latest_base_image_tag(ctx, registry)
        for arch in arches:
            ctx.run(
                f"./tools/host-profiler/prepare-image-context.sh '{BIN_PATH}.{arch}' {LOCAL_IMAGE_BUILD_CONTEXT}",
                echo=True,
            )

            # CI conventions: ddbuild mirror for the builder image, Docker media types on export
            cmd = [
                "docker",
                "buildx",
                "build",
                "--pull",
                f"--platform linux/{arch}",
                f"--build-arg AGENT_BASE_IMAGE_TAG={registry}/ci/datadog-agent/agent-base-image:{base}-{arch}",
                f"--build-arg BASE_IMAGE_REGISTRY={registry}/images/mirror",
                "--output=type=image,push=true,oci-mediatypes=false,compression=gzip,force-compression=true",
                # Provenance attestations would turn each arch tag into a manifest
                # index, which docker manifest create cannot consume.
                "--provenance=false",
                "--target",
                "release",
                "-t",
                f"{image}:{tag}-{arch}",
                LOCAL_IMAGE_BUILD_CONTEXT,
            ]
            if dev_image:
                cmd += ["-t", f"{dev_image}:{tag}-{arch}"]
            print(color_message(f"=== Building and pushing {image}:{tag} for {arch} (base image {base})", "green"))
            ctx.run(" ".join(cmd), echo=True)

        # Push one manifest per registry; the children must live in the destination repository.
        for destination in [image] + ([dev_image] if dev_image else []):
            print(color_message(f"=== Publishing multi-arch manifest {destination}:{tag}", "green"))
            ctx.run(
                f"docker manifest create {destination}:{tag} {destination}:{tag}-amd64 {destination}:{tag}-arm64",
                echo=True,
            )
            # --purge drops the local manifest list, so re-publishing the same tag works.
            ctx.run(f"docker manifest push --purge {destination}:{tag}", echo=True)
    finally:
        for dst in context_files:
            path = os.path.join(LOCAL_IMAGE_BUILD_CONTEXT, dst)
            if os.path.exists(path):
                os.remove(path)

    # Leave the per-arch tags (<tag>-amd64/-arm64) in place: deleting them deletes
    # the child manifests the multi-arch lists reference.

    print(color_message(f"Image pushed: {image}:{tag}", "green"))
    if dev_image:
        print(color_message(f"Image pushed: {dev_image}:{tag}", "green"))
    print("Deploy it by setting config.hostProfiler.image.tag in the KDAO experimental values.")


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
