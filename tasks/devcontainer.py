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

from tasks.build_tags import build_tags, filter_incompatible_tags, get_build_tags, get_default_build_tags
from tasks.commands.docker import AGENT_REPOSITORY_PATH, DockerCLI
from tasks.flavor import AgentFlavor
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.utils import is_installed

DEVCONTAINER_DIR = ".devcontainer"
DEVCONTAINER_FILE = "devcontainer.json"
DEVCONTAINER_NAME = "datadog_agent_devcontainer"
DEVCONTAINER_IMAGE = "registry.ddbuild.io/ci/datadog-agent-devenv:1-arm64"


class SkaffoldProfile(Enum):
    KIND = "kind"
    MINIKUBE = "minikube"


@task
def setup(
    _,
    target="agent",
    build_include=None,
    build_exclude=None,
    SkaffoldProfile=None,
    flavor=AgentFlavor.base.name,
    image='',
):
    """
    Generate or Modify devcontainer settings file for this project.
    """
    flavor = AgentFlavor[flavor]
    if target not in build_tags[flavor]:
        print("Must choose a valid target.  Valid targets are: \n")
        print(f'{", ".join(build_tags[flavor].keys())} \n')
        return

    build_include = (
        get_default_build_tags(build=target, flavor=flavor, platform='linux')
        if build_include is None
        else filter_incompatible_tags(build_include.split(","))
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    use_tags = get_build_tags(build_include, build_exclude)
    use_tags.append("test")  # always include the test tag for autocompletion in vscode

    if not os.path.exists(DEVCONTAINER_DIR):
        os.makedirs(DEVCONTAINER_DIR)

    devcontainer = {}
    fullpath = os.path.join(DEVCONTAINER_DIR, DEVCONTAINER_FILE)
    if os.path.exists(fullpath):
        with open(fullpath) as sf:
            devcontainer = json.load(sf, object_pairs_hook=OrderedDict)

    local_build_tags = ",".join(use_tags)

    devcontainer["name"] = "Datadog-Agent-DevEnv"
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
        "/workspaces/datadog-agent",
        "--name",
        "datadog_agent_devcontainer",
    ]
    devcontainer["features"] = {}
    devcontainer["remoteUser"] = "datadog"
    devcontainer["mounts"] = [
        "source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind,consistency=cached",
        "source=${localEnv:HOME}/.ssh,target=/home/vscode/.ssh,type=bind,consistency=cached",
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

    # onCreateCommond runs the install-tools and deps tasks only when the devcontainer is created and not each time
    # the container is started
    devcontainer["onCreateCommand"] = (
        f"git config --global --add safe.directory {AGENT_REPOSITORY_PATH} && invoke -e install-tools && invoke -e deps"
    )

    devcontainer["containerEnv"] = {
        "GITLAB_TOKEN": "${localEnv:GITLAB_TOKEN}",
    }

    configure_skaffold(devcontainer, SkaffoldProfile(SkaffoldProfile))

    with open(fullpath, "w") as sf:
        json.dump(devcontainer, sf, indent=4, sort_keys=False, separators=(',', ': '))


def configure_skaffold(devcontainer: dict, profile: SkaffoldProfile):
    if profile == SkaffoldProfile.KIND:
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
        }
        devcontainer["customizations"]["vscode"]["settings"].update(additional_settings)

        # add envvars to deploy the agent
        additional_envvars = {
            "DD_API_KEY": "${localEnv:DD_API_KEY}",
            "DD_APP_KEY": "${localEnv:DD_APP_KEY}",
        }
        devcontainer["containerEnv"].update(additional_envvars)

        # add Datadog helm chart registry to the devcontainer
        devcontainer["onCreateCommand"] += " && helm repo add datadog https://helm.datadoghq.com && helm repo update"

    elif profile == SkaffoldProfile.MINIKUBE:
        # TODO: add minikube specific settings
        pass


@task
def start(ctx, path="."):
    """
    Start the devcontainer
    """
    if not file().exists():
        print(color_message("No devcontainer settings found.  Run `invoke devcontainer.setup` first.", Color.RED))
        raise Exit(code=1)

    if not is_installed("devcontainer"):
        print(
            color_message("Devcontainer CLI is not installed.  Run `invoke install-devcontainer-cli` first.", Color.RED)
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
            color_message("No devcontainer settings found. Run `inv devcontainer.setup` first and start it.", Color.RED)
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
