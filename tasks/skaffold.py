"""
Skaffold related tasks
"""

from pathlib import Path

from invoke import UnexpectedExit, task
from invoke.exceptions import Exit

from tasks.devcontainer import DEVCONTAINER_IMAGE, DEVCONTAINER_NAME
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.utils import is_installed

DATADOG_AGENT_MOUNT = "/home/datadog/go/src/github.com/DataDog/datadog-agent"


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


def generate_minikube_env(ctx) -> list:
    """
    Generate the Minikube environment variables
    """
    minikube_env = []
    minikube_env_command = ctx.run("minikube docker-env", hide=True)
    for line in minikube_env_command.stdout.split("\n"):
        if line.startswith("export"):
            minikube_env.append(line.replace("export ", ""))
    return minikube_env


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

    ctx.run(" ".join(generate_minikube_env(ctx) + docker_command))


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
    devcontainer_status = ctx.run(" ".join(generate_minikube_env(ctx) + command), hide=True)
    return devcontainer_status.ok and DEVCONTAINER_NAME in devcontainer_status.stdout


@task
def create(ctx, path=".") -> None:
    """
    Start the Minikube Cluster and the devcontainer
    """
    if not is_minikube_running(ctx):
        minikube_start(ctx, path)
    if not is_devcontainer_running(ctx):
        devcontainer_start(ctx)

    # Skaffold helm deployement will include '.skaffold/values.yaml'
    # so it must at least exists, it can be empty or filled by the user
    helm_values = Path(".skaffold/values.yaml")
    if not helm_values.exists():
        helm_values.touch()


@task(
    help={
        "tail": "Tail all containers logs if set (very verbose, tailing all agent processes). Default False",
        "log_level": "Skaffold log level (matches 'logrus' levels). Default warn",
    },
)
def dev(ctx, tail: bool = False, log_level: str = "warn") -> None:
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

    # Create Skaffold Dev command
    skaffold_command = [
        "skaffold",
        "dev",
        "--auto-build=false",
        "--auto-deploy=false",
        "--auto-sync=false",
        "--port-forward=off",
        f"--verbosity {log_level}",
        f"--tail={tail}",
    ]

    if is_minikube_running(ctx):
        ctx.run(" ".join(generate_minikube_env(ctx) + skaffold_command))
    else:
        ctx.run(" ".join(skaffold_command))
