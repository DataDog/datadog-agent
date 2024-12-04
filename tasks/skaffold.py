"""
Skaffold related tasks
"""

from invoke import task
from invoke.exceptions import Exit

from tasks.devcontainer import DEVCONTAINER_NAME, DEVCONTAINER_IMAGE
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.utils import is_installed

DATADOG_AGENT_MOUNT = "/home/datadog/go/src/github.com/DataDog/datadog-agent"

@task
def minikube_start(ctx, path="."):
    """
    Start the Minikube Cluster
    """
    if not is_installed(ctx, "minikube"):
        print(
            color_message("Minikube CLI is not installed. Check https://minikube.sigs.k8s.io/docs/start", Color.RED)
        )
        raise Exit(code=1)

    ctx.run(f"minikube start --mount --mount-string {path}:{DATADOG_AGENT_MOUNT}")

@task
def devcontainer_start(ctx):
    """
    Start the devcontainer
    """
    if not is_installed(ctx, "docker"):
        print(
            color_message("Docker CLI is not installed. Check https://docs.docker.com/desktop", Color.RED)
        )
        raise Exit(code=1)

    # Get the minikube env variables
    minikube_env = []
    minikube_env_command =  ctx.run("minikube docker-env", hide=True)
    for line in minikube_env_command.stdout.split("\n"):
        if line.startswith("export"):
            minikube_env.append(line.replace("export ", ""))

    print(
        color_message("To see the running containers in Minikube, run the following command: eval $(minikube docker-env)", Color.GREEN)
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
        f"--user root",
        f"{DEVCONTAINER_IMAGE}",
        "sleep infinity",
    ]

    ctx.run(" ".join(minikube_env + docker_command))

@task
def create(ctx, path="."):
    """
    Start the Minikube Cluster and the devcontainer
    """
    minikube_start(ctx, path)
    devcontainer_start(ctx)

@task
def dev(ctx):
    """
    Start the Skaffold cluster
    """
    if not is_installed(ctx, "skaffold"):
        print(
            color_message("Skaffold is not installed. Check https://skaffold.dev/docs/install/#standalone-binary", Color.RED)
        )
        raise Exit(code=1)

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
    ctx.run(" ".join(skaffold_command))
