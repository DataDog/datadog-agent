"""
Helpers for working with devcontainers
"""

import json
import os
import platform as py_platform
import sys
from collections import OrderedDict
from enum import Enum
from functools import wraps
from pathlib import Path

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import build_tags, compute_build_tags_for_flavor
from tasks.commands.docker import DockerCLI
from tasks.flavor import AgentFlavor
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.utils import is_installed

DEVCONTAINER_DIR = ".devcontainer"
DEVCONTAINER_FILE = "devcontainer.json"
DEVCONTAINER_NAME = "datadog-agent-devcontainer"
DEVCONTAINER_IMAGE = "registry.ddbuild.io/ci/datadog-agent-devenv:1-arm64"


class SkaffoldProfile(Enum):
    KIND = "kind"
    MINIKUBE = "minikube"
    NONE = None


@task
def setup(
    _,
    target="agent",
    build_include=None,
    build_exclude=None,
    skaffoldProfile=None,
    flavor=AgentFlavor.base.name,
    image='',
    claude_code=False,
):
    """
    Generate or Modify devcontainer settings file for this project.
    """
    flavor = AgentFlavor[flavor]
    if target not in build_tags[flavor]:
        print("Must choose a valid target.  Valid targets are: \n")
        print(f'{", ".join(build_tags[flavor].keys())} \n')
        return

    use_tags = compute_build_tags_for_flavor(
        build=target, flavor=flavor, build_include=build_include, build_exclude=build_exclude, platform='linux'
    )
    use_tags.append("test")  # always include the test tag for autocompletion in vscode

    if not os.path.exists(DEVCONTAINER_DIR):
        os.makedirs(DEVCONTAINER_DIR)

    devcontainer = {}
    fullpath = os.path.join(DEVCONTAINER_DIR, DEVCONTAINER_FILE)
    if os.path.exists(fullpath):
        with open(fullpath) as sf:
            devcontainer = json.load(sf, object_pairs_hook=OrderedDict)

    local_build_tags = ",".join(use_tags)

    devcontainer["name"] = "Datadog Agent Development Container"
    if image:
        devcontainer["image"] = image
        if devcontainer.get("build"):
            del devcontainer["build"]
    else:
        devcontainer["build"] = {
            "dockerfile": "Dockerfile",
            "args": {},
        }
        if devcontainer.get("image"):
            del devcontainer["image"]
    devcontainer["runArgs"] = [
        "--cap-add=SYS_PTRACE",
        "--security-opt",
        "seccomp=unconfined",
        "-w",
        "/workspaces/${localWorkspaceFolderBasename}",
        "--name",
        "datadog-agent-devcontainer",
    ]
    devcontainer["features"] = {}
    devcontainer["remoteUser"] = "datadog"
    devcontainer["mounts"] = [
        "source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind",
        "source=${localEnv:HOME}/.ssh,target=/home/datadog/.ssh,type=bind",
    ]
    devcontainer["customizations"] = {
        "vscode": {
            "settings": {
                "go.toolsManagement.checkForUpdates": "local",
                "go.useLanguageServer": True,
                "go.gopath": "/home/datadog/go",
                "go.goroot": "/usr/local/go",
                "go.buildTags": local_build_tags,
                "go.testTags": local_build_tags,
                "go.lintTool": "golangci-lint",
                "go.lintOnSave": "package",
                "go.lintFlags": [
                    "--build-tags",
                    local_build_tags,
                ],
                "[go]": {
                    "editor.formatOnSave": True,
                },
                "gopls": {"formatting.local": "github.com/DataDog/datadog-agent"},
            },
            "extensions": ["golang.Go", "ms-python.python", "redhat.vscode-yaml"],
        }
    }

    # onCreateCommand runs the install-tools and deps tasks only when the devcontainer is created and not each time
    # the container is started
    devcontainer["onCreateCommand"] = (
        "git config --global --add safe.directory /workspaces/${localWorkspaceFolderBasename} && dda inv -- -e install-tools && dda inv -- -e deps"
    )

    devcontainer["containerEnv"] = {
        "GITLAB_TOKEN": "${localEnv:GITLAB_TOKEN}",
    }

    configure_skaffold(devcontainer, SkaffoldProfile(skaffoldProfile))
    configure_claude_code(devcontainer, claude_code)

    # Skaffold helm deployement will include '.skaffold/values.yaml'
    # so it must at least exists, it can be empty or filled by the user
    if skaffoldProfile is not None:
        helm_values = Path(".skaffold/values.yaml")
        if not helm_values.exists():
            helm_values.touch()

    # Add per user configuration
    user_config_path = Path.home() / ".devcontainer" / "agent_overrides.json"
    if os.path.exists(user_config_path):
        with open(user_config_path) as sf:
            user_config = json.load(sf)
            more_mounts = user_config.get("mounts")
            if more_mounts:
                devcontainer["mounts"].append(more_mounts)
            more_create = user_config.get("onCreate")
            if more_create:
                devcontainer["onCreateCommand"] = devcontainer["onCreateCommand"] + " && " + " && ".join(more_create)

    with open(fullpath, "w") as sf:
        json.dump(devcontainer, sf, indent=4, sort_keys=False, separators=(',', ': '))


def configure_claude_code(devcontainer: dict, claude_code: bool):
    if claude_code:
        # create folder .devcontainer/claude-data/.claude if not exists
        claude_data_path = Path.home() / ".devcontainer" / "claude-data"
        Path(claude_data_path).mkdir(parents=True, exist_ok=True)
        devcontainer["mounts"].append(
            "source=${localWorkspaceFolder}/.devcontainer/claude-data/,target=/home/datadog/.claude,type=bind"
        )

        devcontainer["features"]["ghcr.io/devcontainers/features/node:1"] = {}
        devcontainer["features"]["ghcr.io/anthropics/devcontainer-features/claude-code:1.0"] = {}


def configure_skaffold(devcontainer: dict, profile: SkaffoldProfile):
    match profile:
        case SkaffoldProfile.KIND:
            devcontainer["runArgs"].append("--network=host")  # to connect to the kind api-server
            # add requires extensions
            additional_extensions = ["GoogleCloudTools.cloudcode"]
            devcontainer["customizations"]["vscode"]["extensions"].extend(additional_extensions)

            # Additionnal features
            additional_features = {
                "ghcr.io/rio/features/skaffold:2": {},
                "ghcr.io/devcontainers/features/kubectl-helm-minikube:1": {},
                "ghcr.io/devcontainers-extra/features/kind:1": {},
                "ghcr.io/dhoeric/features/google-cloud-cli:1": {},
            }
            devcontainer["features"].update(additional_features)

            # Addionnal settings
            additional_settings = {
                "cloudcode.features.completion": False,
                "cloudcode.ai.assistance.enabled": False,
                "cloudcode.cloudsdk.checkForMissing": False,
                "cloudcode.cloudsdk.autoInstall": False,
                "cloudcode.autoDependencies": "off",
                "cloudcode.enableGkeAutopilotSupport": False,
                "cloudcode.enableMinikubeGcpAuthPlugin": False,
                "cloudcode.enableTelemetry": False,
                "cloudcode.updateAdcOnLogin": False,
                "cloudcode.useGcloudAuthSkaffold": False,
                "cloudcode.yaml.validate": False,
                # TODO: for now we need to keep it else the cloudrun plugin is broken
                # "geminicodeassist.enable": False,
                "geminicodeassist.enableTelemetry": False,
            }
            devcontainer["customizations"]["vscode"]["settings"].update(additional_settings)

            # add envvars to deploy the agent
            additional_envvars = {
                "DD_API_KEY": "${localEnv:DD_API_KEY}",
                "DD_APP_KEY": "${localEnv:DD_APP_KEY}",
            }
            devcontainer["containerEnv"].update(additional_envvars)

            # add Datadog helm chart registry to the devcontainer
            devcontainer["onCreateCommand"] += (
                " && helm repo add datadog https://helm.datadoghq.com && helm repo update"
            )
        case SkaffoldProfile.MINIKUBE:
            # TODO: add minikube specific settings
            pass
        case SkaffoldProfile.NONE:
            # Nothing to do in case of none
            pass


@task
def start(ctx, path="."):
    """
    Start the devcontainer
    """
    if not file().exists():
        print(color_message("No devcontainer settings found.  Run `dda inv devcontainer.setup` first.", Color.RED))
        raise Exit(code=1)

    if not is_installed("devcontainer"):
        print(
            color_message(
                "Devcontainer CLI is not installed.  Run `dda inv install-devcontainer-cli` first.", Color.RED
            )
        )
        raise Exit(code=1)

    ctx.run(f"devcontainer up --workspace-folder {path}")


@task
def stop(ctx):
    """
    Stop the running devcontainer
    """
    if not file().exists():
        print(
            color_message(
                "No devcontainer settings found. Run `dda inv devcontainer.setup` first and start it.", Color.RED
            )
        )
        raise Exit(code=1)

    if not is_up(ctx):
        print(color_message("Devcontainer is not running.", Color.RED))
        raise Exit(code=1)

    ctx.run(f"docker kill {DEVCONTAINER_NAME}")


@task
def restart(ctx, path="."):
    """
    Restart the devcontainer
    """
    ctx.run(f"docker rm -f {DEVCONTAINER_NAME}")
    start(ctx, path)


def file() -> Path:
    return Path(DEVCONTAINER_DIR) / DEVCONTAINER_FILE


def is_up(ctx) -> bool:
    res = ctx.run("docker ps", hide=True, warn=True)
    # TODO: it's fragile to just check for the container name, but it's the best we can do for now
    return DEVCONTAINER_NAME in res.stdout


def run_on_devcontainer(func):
    """
    This decorator will run the decorated function in a devcontainer if the selected platform is linux.
    All you need to do is to decorate your task with this decorator and add a `platform` argument to your task.
    """

    @wraps(func)
    def _run_on_devcontainer(ctx, *args, **kwargs):
        if kwargs.get('run_on'):
            platform = kwargs['run_on'].lower()
            if platform == "linux" and py_platform.system().lower() != platform:
                # If we choose to run them on linux, and we are not on linux already
                if not file().exists():
                    print(
                        color_message(
                            "Generating the devcontainer file to run the linter in a container.", Color.ORANGE
                        )
                    )
                    # TODO remove the hardcoded image and auto-pull it
                    setup(ctx, image=DEVCONTAINER_IMAGE)

                if not is_up(ctx):
                    print(color_message("Starting the devcontainer...", Color.ORANGE))
                    start(ctx)

                print(color_message("Running the command in the devcontainer...", Color.ORANGE))
                cli = DockerCLI(DEVCONTAINER_NAME)

                cmd = ["inv"] + sys.argv[1:]
                if not cli.run_command(cmd).ok:
                    print(color_message("Failed to run the command in the devcontainer.", Color.RED))
                    raise Exit(code=1)

                return

        func(ctx, *args, **kwargs)

    return _run_on_devcontainer
