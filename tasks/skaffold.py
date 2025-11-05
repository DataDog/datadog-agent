"""
Skaffold related tasks
"""

from invoke import UnexpectedExit, task
from invoke.exceptions import Exit

from tasks.devcontainer import DEVCONTAINER_IMAGE, DEVCONTAINER_NAME, SkaffoldProfile
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


def is_kind_running(ctx) -> bool:
    """
    Check if kind is running
    """
    if not is_installed("kind"):
        return False

    try:
        kind_status = ctx.run("kind get clusters", hide=True, warn=True)
    except UnexpectedExit:
        if kind_status.return_code == 1:
            return False

        # in case of any other error raise.
        raise

    if kind_status.stdout.find("No kind clusters found.") == 0:
        return False

    return True


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


@task(
    help={
        "profile": "The skaffold profile to tuse (minikube, kind, devcontainer)",
        "tail": "Tail all containers logs if set (very verbose, tailing all agent processes). Default False",
        "logLevel": "Skaffold log leve (matches 'logrus' levels). Default warn",
    },
)
def dev(ctx, profile: str, tail: bool = False, logLevel: str = "warn") -> None:
    """
    Start the Skaffold cluster
    """

    # NONE is not a valid profile
    skaffold_valid_profiles = [name.lower() for name in SkaffoldProfile._member_names_ if name != 'NONE']
    if profile == "" or profile not in skaffold_valid_profiles:
        print(
            color_message(
                f"{profile} not valid, please select a profile from: {skaffold_valid_profiles}",
                Color.RED,
            )
        )
        raise Exit(code=1)

    skaffold_profile = SkaffoldProfile(profile)

    print(
        color_message(
            f"Starting the Skaffold cluster, profile: {skaffold_profile.value}",
            Color.GREEN,
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

    match skaffold_profile:
        case SkaffoldProfile.MINIKUBE:
            # Create Minikube Cluster and devcontainer if they are not running.
            create(ctx)

        case SkaffoldProfile.KIND | SkaffoldProfile.DEVCONTAINER:
            if not is_kind_running(ctx):
                print(
                    color_message(
                        "Ensure your kind cluster is started and healthy",
                        Color.RED,
                    )
                )

                raise Exit(code=1)

        case _:
            print(
                color_message(
                    f"{skaffold_profile.value} not valid, please select a profile from: {skaffold_valid_profiles}",
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
        f"--verbosity {logLevel}",
        f"--tail={tail}",
        f"--profile={skaffold_profile.value}",
    ]

    if skaffold_profile == SkaffoldProfile.MINIKUBE:
        ctx.run(" ".join(generate_minikube_env(ctx) + skaffold_command))
    else:
        ctx.run(" ".join(skaffold_command))
