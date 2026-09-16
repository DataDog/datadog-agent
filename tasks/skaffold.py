"""
Skaffold related tasks
"""

import os

from invoke import UnexpectedExit, task
from invoke.exceptions import Exit

from tasks.devcontainer import DEVCONTAINER_IMAGE, DEVCONTAINER_NAME
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.utils import is_installed

DATADOG_AGENT_MOUNT = "/home/datadog/go/src/github.com/DataDog/datadog-agent"

# Maps a Skaffold artifact name to the invoke task that builds its development image.
SKAFFOLD_BUILD_TASKS = {
    "agent": "agent.hacky-dev-image-build",
    "clusteragent": "cluster-agent.hacky-dev-image-build",
}


@task
def minikube_start(ctx, path: str = ".") -> None:
    """
    Start the Minikube Cluster
    """
    print(
        color_message(
            "Starting the Minikube cluster.",
            Color.BLUE,
        )
    )
    if not is_installed("minikube"):
        print(
            color_message(
                "Minikube CLI is not installed. Check https://minikube.sigs.k8s.io/docs/start",
                Color.RED,
            )
        )
        raise Exit(code=1)

    ctx.run(f"minikube start --mount --mount-string {path}:{DATADOG_AGENT_MOUNT}")


def is_minikube_running(ctx) -> bool:
    """
    Check if Minikube is running
    """
    try:
        minikube_status = ctx.run("minikube status", hide=True, warn=True)
    except UnexpectedExit:
        if minikube_status.return_code == 130:
            return False
        else:
            raise

    return minikube_status.ok


def minikube_docker_env(ctx) -> dict:
    """
    Return env vars pointing the Docker CLI at Minikube's daemon, for `ctx.run(env=...)`.
    """
    # Shell-agnostic `KEY=value` pairs for easier parsing
    result = ctx.run("minikube docker-env --shell none", hide=True)
    env = {}
    for line in result.stdout.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        key, sep, value = line.partition("=")
        value = value.strip().strip('"')
        # Skip empty assignments (e.g. `SSH_AUTH_SOCK=`) so they don't clobber inherited values.
        if sep and value:
            env[key.strip()] = value
    return env


@task
def devcontainer_start(ctx) -> None:
    """
    Start the devcontainer
    """
    print(
        color_message(
            "Starting the devcontainer.",
            Color.BLUE,
        )
    )
    if not is_installed("docker"):
        print(
            color_message(
                "Docker CLI is not installed. Check https://docs.docker.com/desktop.",
                Color.RED,
            )
        )
        raise Exit(code=1)

    print(
        color_message(
            "To see the running containers in Minikube, run the following command: eval $(minikube docker-env).",
            Color.GREEN,
        )
    )

    # Create Docker command
    docker_command = [
        "docker",
        "run",
        "-d",
        f"-v {DATADOG_AGENT_MOUNT}:{DATADOG_AGENT_MOUNT}",
        "-v /var/run/docker.sock:/var/run/docker.sock",
        f"--name {DEVCONTAINER_NAME}",
        "--pull missing",
        f"-w {DATADOG_AGENT_MOUNT}",
        "--user root",
        f"{DEVCONTAINER_IMAGE}",
        "sleep infinity",
    ]

    ctx.run(" ".join(docker_command), env=minikube_docker_env(ctx))


def is_devcontainer_running(ctx) -> bool:
    """
    Check if the devcontainer is running
    """
    command = [
        "docker",
        "ps",
        "--filter",
        f"name={DEVCONTAINER_NAME}",
        "--format",
        "{{.Names}}",
    ]
    devcontainer_status = ctx.run(" ".join(command), hide=True, env=minikube_docker_env(ctx))
    return devcontainer_status.ok and DEVCONTAINER_NAME in devcontainer_status.stdout


def ensure_not_worktree(path: str) -> None:
    """
    Exit if `path` is a git worktree.
    """
    if os.path.isfile(os.path.join(path, ".git")):
        print(
            color_message(
                f"{os.path.abspath(path)} is a git worktree, which the Skaffold development flow does not "
                "support: only this directory is mounted into the build container, but a worktree's git "
                "metadata lives in the main repository outside the mount. Run this from a normal clone instead.",
                Color.RED,
            )
        )
        raise Exit(code=1)


@task
def create(ctx, path=".") -> None:
    """
    Start the Minikube Cluster and the devcontainer
    """
    ensure_not_worktree(path)
    if not is_minikube_running(ctx):
        minikube_start(ctx, path)
    if not is_devcontainer_running(ctx):
        devcontainer_start(ctx)


@task
def destroy(ctx) -> None:
    """
    Remove the devcontainer and delete the Minikube cluster, including anything deployed to it.
    """
    if is_minikube_running(ctx):
        print(color_message("Removing the devcontainer.", Color.BLUE))
        ctx.run(f"docker rm -f {DEVCONTAINER_NAME}", warn=True, env=minikube_docker_env(ctx))
    print(color_message("Deleting the Minikube cluster.", Color.BLUE))
    ctx.run("minikube delete")


@task
def dev(ctx) -> None:
    """
    Start the Skaffold cluster
    """
    print(
        color_message(
            "Starting the Skaffold cluster.",
            Color.BLUE,
        )
    )
    # Check if Skaffold is installed
    if not is_installed("skaffold"):
        print(
            color_message(
                "Skaffold is not installed. Check https://skaffold.dev/docs/install/#standalone-binary.",
                Color.RED,
            )
        )
        raise Exit(code=1)

    # Create Minikube Cluster and devcontainer if they are not running.
    create(ctx)

    # The deploy installs the `datadog/datadog` remote chart, so ensure its Helm repo is
    # registered and its index cache (kept under a temp dir that may not persist) is fresh.
    if not is_installed("helm"):
        print(color_message("Helm is not installed. Check https://helm.sh/docs/intro/install.", Color.RED))
        raise Exit(code=1)
    ctx.run("helm repo add datadog https://helm.datadoghq.com --force-update", warn=True)
    ctx.run("helm repo update datadog")

    # Create Skaffold Dev command
    skaffold_command = [
        "skaffold",
        "dev",
        "--filename skaffold.yaml",
        "--auto-build=false",
        "--auto-deploy=false",
        "--auto-sync=false",
        "--port-forward=true",
        "--status-check=true",
        "--verbosity warn",
    ]
    ctx.run(" ".join(skaffold_command), env=minikube_docker_env(ctx))


@task
def build(ctx, target="agent") -> None:
    """
    Build a development image in the running devcontainer, for Skaffold's custom builder.

    Assumes that Skaffold has already set the `IMAGE` env var to the target tag.
    """
    build_task = SKAFFOLD_BUILD_TASKS.get(target)
    if build_task is None:
        raise Exit(f"Unknown target '{target}'. Valid targets: {', '.join(SKAFFOLD_BUILD_TASKS)}.", code=1)

    target_image = os.environ.get("IMAGE")
    if not target_image:
        raise Exit("The IMAGE environment variable is not set (it is provided by Skaffold).", code=1)

    # Run via a login shell to ensure proper environment setup.
    inner = f"dda inv -- {build_task} --target-image={target_image}"
    ctx.run(f'docker exec {DEVCONTAINER_NAME} bash -lc "{inner}"', env=minikube_docker_env(ctx))
