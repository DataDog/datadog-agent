from __future__ import annotations

import getpass
import platform
from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app
from dda.cli.env.dev.utils import option_env_type

if TYPE_CHECKING:
    from dda.cli.application import Application

ECR_REGISTRY = "376334461865.dkr.ecr.us-east-1.amazonaws.com"
DDBUILD_REGISTRY = "registry.ddbuild.io"
GITLAB_API = "https://gitlab.ddbuild.io/api/v4"
GITLAB_PROJECT = "DataDog%2Fdatadog-agent"
RELEASE_IMAGE_REPOSITORY = "datadog/agent"


def _get_gitlab_token(app: Application) -> tuple[str, bool]:
    """Get a GitLab token, preferring ddtool OAuth over GITLAB_TOKEN env var.

    Returns (token, is_oauth) where is_oauth indicates how python-gitlab should authenticate.
    """
    import os
    import shutil
    import subprocess

    # Try ddtool OAuth first if it's available
    # Mirror get_gitlab_oauth_token from tasks/libs/ciproviders/gitlab_api.py
    if shutil.which("ddtool"):
        result = app.subprocess.attach(
            ["ddtool", "auth", "gitlab", "token"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            check=False,
        )
        if "ddtool auth gitlab login" in (result.stderr or ""):
            app.display_warning("GitLab OAuth login required. Run `ddtool auth gitlab login` first.")
        elif result.returncode == 0:
            token = result.stdout.strip()
            if len(token) == 64:
                return token, True
            app.display_warning("Unexpected response from `ddtool auth gitlab token`.")

    # Fall back to GITLAB_TOKEN env var (Personal Access Token)
    token = os.environ.get("GITLAB_TOKEN")
    if token:
        return token, False

    app.abort(
        "Could not obtain a GitLab token. "
        "Either run `ddtool auth gitlab login` or set GITLAB_TOKEN to a Personal Access Token with read_api scope."
    )


def _get_latest_main_base_image(app: Application, arch: str) -> str | None:
    """Query GitLab for the latest successful pipeline on main and return the corresponding agent image."""
    # Ensure python-gitlab is available (only needed for this code path)
    try:
        import gitlab
    except ImportError:
        app.abort(
            "python-gitlab is required to fetch the latest base image from main.\n"
            "Specify a base image explicitly with --base-image (e.g., --base-image=7.63.0)\n"
            "or install python-gitlab: dda env dev shell -- pip install python-gitlab"
        )

    token, is_oauth = _get_gitlab_token(app)

    try:
        auth_kwargs = {"oauth_token": token} if is_oauth else {"private_token": token}
        gl = gitlab.Gitlab("https://gitlab.ddbuild.io", **auth_kwargs)
        project = gl.projects.get("DataDog/datadog-agent")
        pipelines = project.pipelines.list(
            ref="main", status="success", per_page=1, order_by="id", sort="desc", get_all=False
        )
        if not pipelines:
            app.display_warning("No successful pipelines found on main.")
            return None

        pipeline = pipelines[0]
        pipeline_id = pipeline.id
        commit_sha = pipeline.sha[:8]
        image = f"{DDBUILD_REGISTRY}/ci/datadog-agent/agent:v{pipeline_id}-{commit_sha}-7-{arch}"
        app.display(f"Using base image from main (pipeline {pipeline_id}): {image}")
        return image
    except Exception as e:
        app.display_warning(f"Could not fetch latest main image: {e}")
        return None


def _build_full_image(app: Application, *, target_image: str, arch: str | None) -> None:
    """Build agent image using full omnibus build (production-like, slower)."""
    # Auto-detect architecture if not specified
    if arch is None:
        arch = "arm64" if platform.machine().lower() in {"arm64", "aarch64"} else "amd64"

    app.display(f"Building agent image using full omnibus build (arch: {arch})")
    build_cmd = ["dda", "inv", "omnibus.docker-build", f"--arch={arch}", f"--tag={target_image}"]
    app.subprocess.run(build_cmd)


def _build_quick_image(
    app: Application,
    *,
    target_image: str,
    base_image: str | None,
    env_type: str,
    instance: str,
    arch: str | None,
) -> None:
    """Build agent image using quick method (fast iteration with dev environment)."""
    from dda.env.dev import get_dev_env
    from dda.env.models import EnvironmentState

    # Step 1: ensure the dev environment is running with the correct arch
    env = get_dev_env(env_type)(app=app, name=env_type, instance=instance)
    status = env.status()

    # If the environment exists with the wrong arch (running or stopped), remove it so it can be
    # recreated with the correct arch — dda env dev start cannot reconfigure a stopped container.
    if status.state in {EnvironmentState.STARTED, EnvironmentState.STOPPED} and arch:
        host_arch = "arm64" if platform.machine().lower() in {"arm64", "aarch64"} else "amd64"
        container_arch = env.config.arch or host_arch
        if arch != container_arch:
            app.display_warning(
                f"Dev environment has arch `{container_arch}` but `--arch {arch}` was requested. "
                f"Removing and recreating with the correct arch..."
            )
            env_type_args = ["--type", env_type] if env_type else []
            instance_args = ["--id", instance] if instance != "default" else []
            if status.state == EnvironmentState.STARTED:
                app.subprocess.run(["dda", "env", "dev", "stop", *env_type_args, *instance_args])
            app.subprocess.run(["dda", "env", "dev", "remove", *env_type_args, *instance_args])
            env = get_dev_env(env_type)(app=app, name=env_type, instance=instance)
            status = env.status()

    if status.state in {EnvironmentState.NONEXISTENT, EnvironmentState.STOPPED}:
        app.display_warning(f"Developer environment `{env_type}` is not running — starting it now...")
        start_cmd = ["dda", "env", "dev", "start"]
        # --type must come before --arch since --arch is a dynamic option
        # registered after --type resolves the environment class
        if env_type:
            start_cmd.extend(["--type", env_type])
        if arch:
            start_cmd.extend(["--arch", arch])
        if instance != "default":
            start_cmd.extend(["--id", instance])
        app.subprocess.run(start_cmd)
    elif status.state != EnvironmentState.STARTED:
        app.abort(
            f"Developer environment `{env_type}` is in state `{status.state.value}` and cannot be started automatically. "
            f"Please resolve the issue manually and try again."
        )

    # Determine the effective arch from the running container's config.
    # hacky_dev_image_build auto-detects arch from platform.machine() inside the container,
    # which reflects the arch the container was started with — cross-compilation within a
    # running container is not supported.
    env = get_dev_env(env_type)(app=app, name=env_type, instance=instance)
    host_arch = "arm64" if platform.machine().lower() in {"arm64", "aarch64"} else "amd64"
    effective_arch = env.config.arch or host_arch

    # Resolve the base image
    resolved_base_image = base_image
    if resolved_base_image is None:
        resolved_base_image = _get_latest_main_base_image(app, effective_arch)
        if resolved_base_image and DDBUILD_REGISTRY in resolved_base_image:
            app.display_warning(
                f"Using private registry image: {resolved_base_image}\n"
                f"If you don't have access to {DDBUILD_REGISTRY}, the build may fail.\n"
                f"Use --base-image with a version (e.g., --base-image=7.63.0) to use a public image instead."
            )
    elif "/" not in resolved_base_image and ":" not in resolved_base_image:
        # Bare version string (e.g. "7.63.0") — validate and resolve to the public release image.
        import re

        if not re.fullmatch(r"7\.\d+\.\d+(-\d+)?", resolved_base_image):
            app.abort(f"Invalid agent version: '{resolved_base_image}'. Expected format: 7.X.Y (e.g. '7.63.0').")
        resolved_base_image = f"{RELEASE_IMAGE_REPOSITORY}:{resolved_base_image}"

    # Step 2: clean stale CMake cache to avoid source-path mismatch errors, then build
    env.run_command(["dda", "inv", "rtloader.clean"])
    build_cmd = ["dda", "inv", "agent.hacky-dev-image-build", "--target-image", target_image]
    if resolved_base_image:
        build_cmd.extend(["--base-image", resolved_base_image])

    app.display(f"Building agent image: {target_image}")
    env.run_command(build_cmd)


@dynamic_command(
    short_help="Build an Agent Docker image and push it to the sandbox ECR registry for E2E testing",
)
@click.option(
    "--tag",
    default=None,
    help="Tag for the image (default: your username). The image will be pushed to <registry>/<repository>:<tag>",
)
@click.option(
    "--registry",
    default=ECR_REGISTRY,
    show_default=True,
    help="Docker registry to push the image to. Automated login via aws-vault is only supported for the default agent-sandbox ECR registry — for any other registry you must authenticate manually first with `docker login <registry>`.",
)
@click.option(
    "--base-image",
    default=None,
    help=(
        "Base agent image to build from (only used with --build-method=quick). "
        "Accepts a version (e.g. '7.63.0', resolved to datadog/agent:7.63.0) or a full image reference. "
        "Defaults to the latest successful build from main on registry.ddbuild.io."
    ),
)
@click.option(
    "--build-method",
    type=click.Choice(["quick", "full"]),
    default="quick",
    show_default=True,
    help=(
        "Build method: 'quick' (default) uses agent.hacky-dev-image-build for fast iteration (~1-2 min), "
        "layers local binaries on a base image. 'full' uses omnibus.docker-build for production-like "
        "builds from scratch (~10-30 min), includes full dependency compilation."
    ),
)
@option_env_type()
@click.option(
    "--id", "instance", default="default", show_default=True, help="Unique identifier for the dev environment"
)
@click.option(
    "--arch",
    default=None,
    type=click.Choice(["amd64", "arm64"]),
    help="Architecture of the image to build. Only used when the dev environment needs to be started.",
)
@click.option(
    "--no-push",
    is_flag=True,
    default=False,
    help="Build the image locally without pushing to the registry.",
)
@pass_app
def cmd(
    app: Application,
    *,
    tag: str | None,
    registry: str,
    base_image: str | None,
    build_method: str,
    env_type: str,
    instance: str,
    arch: str | None,
    no_push: bool,
) -> None:
    """
    Build an Agent Docker image and push it to the agent-sandbox ECR registry, used in E2E testing local runs.

    Two build methods are supported:
    - 'quick' (default): Fast iteration using agent.hacky-dev-image-build (~1-2 min)
    - 'full': Production-like build using omnibus.docker-build (~10-30 min)

    Quick method workflow:
    1. Ensures the dev environment is running (starts it if needed).
    2. Builds the Agent binary and Docker image via `dda inv agent.hacky-dev-image-build`.
    3. Logs in to the agent-sandbox ECR registry using aws-vault.
    4. Pushes the image to `376334461865.dkr.ecr.us-east-1.amazonaws.com/agent-e2e-tests`.

    Full method workflow:
    1. Builds the Agent using full omnibus build inside a Docker container.
    2. Creates a Docker image from the omnibus build output.
    3. Logs in to the registry and pushes the image.

    The resulting image can be used with the `--agent-image` flag of `dda inv new-e2e-tests.run`.

    Note: images in the agent-sandbox registry are deleted after two days.
    """
    import time

    start_time = time.perf_counter()
    repository = f"{registry}/agent-e2e-tests"
    target_image = f"{repository}:{tag or getpass.getuser()}"

    # Build the image using the selected method
    if build_method == "full":
        _build_full_image(app, target_image=target_image, arch=arch)
    else:
        _build_quick_image(
            app,
            target_image=target_image,
            base_image=base_image,
            env_type=env_type,
            instance=instance,
            arch=arch,
        )

    if no_push:
        elapsed = time.perf_counter() - start_time
        app.display(f"Image built locally in {elapsed:.0f}s: {target_image}")
        app.display(
            "Note: The image was not pushed to a registry. "
            "To use it with E2E tests, either:\n"
            "  1. Push it manually: docker push " + target_image + "\n"
            "  2. Load it into a local cluster: docker save "
            + target_image
            + " | kind load docker-image "
            + target_image
        )
        return

    # Step 3: log in to the registry.
    # Automated login via aws-vault is only supported for the default sandbox ECR registry.
    # For a custom --registry the user is expected to be already authenticated.
    if registry == ECR_REGISTRY:
        app.display(f"Logging in to ECR registry: {registry}")
        ecr_password = app.subprocess.capture(
            [
                "aws-vault",
                "exec",
                "sso-agent-sandbox-account-admin",
                "--",
                "aws",
                "ecr",
                "get-login-password",
                "--region",
                "us-east-1",
            ]
        )
        app.subprocess.attach(
            ["docker", "login", "--username", "AWS", "--password-stdin", registry],
            input=ecr_password,
            text=True,
        )
    else:
        app.display(
            f"Custom registry detected — skipping automated login. Ensure you are already authenticated to {registry}."
        )

    # Step 4: push the image
    app.display(f"Pushing image: {target_image}")
    app.subprocess.run(["docker", "push", target_image])

    elapsed = time.perf_counter() - start_time
    app.display(f"Done in {elapsed:.0f}s. Image available at: {target_image}")
    app.display(f"Run E2E tests with: dda inv new-e2e-tests.run --agent-image {target_image} ...")
