"""
Helpers for working with devcontainers
"""

import json
import os
import platform as py_platform
import sys
from collections import OrderedDict
from functools import wraps
from pathlib import Path

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import build_tags, filter_incompatible_tags, get_build_tags, get_default_build_tags
from tasks.commands.docker import AGENT_REPOSITORY_PATH, DockerCLI
from tasks.flavor import AgentFlavor
from tasks.libs.common.color import color_message

DEVCONTAINER_DIR = ".devcontainer"
DEVCONTAINER_FILE = "devcontainer.json"
DEVCONTAINER_NAME = "datadog_agent_devcontainer"
DEVCONTAINER_IMAGE = "486234852809.dkr.ecr.us-east-1.amazonaws.com/ci/datadog-agent-devenv:1-arm64"


@task
def setup(
    _,
    target="agent",
    build_include=None,
    build_exclude=None,
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
        get_default_build_tags(build=target, flavor=flavor)
        if build_include is None
        else filter_incompatible_tags(build_include.split(","))
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    use_tags = get_build_tags(build_include, build_exclude)

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
        "--name",
        "datadog_agent_devcontainer",
    ]
    devcontainer["remoteUser"] = "datadog"
    devcontainer["mounts"] = ["source=/var/run/docker.sock,target=/var/run/docker.sock,type=bind,consistency=cached"]
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
                "go.lintOnSave": "file",
                "go.lintFlags": [
                    "--build-tags",
                    local_build_tags,
                    "--config",
                    f"{AGENT_REPOSITORY_PATH}/.golangci.yml",
                ],
                "[go]": {
                    "editor.formatOnSave": True,
                },
                "gopls": {"formatting.local": "github.com/DataDog/datadog-agent"},
            },
            "extensions": ["golang.Go"],
        }
    }
    devcontainer["postStartCommand"] = (
        f"git config --global --add safe.directory {AGENT_REPOSITORY_PATH} && invoke install-tools && invoke deps"
    )

    with open(fullpath, "w") as sf:
        json.dump(devcontainer, sf, indent=4, sort_keys=False, separators=(',', ': '))


@task
def start(ctx, path="."):
    """
    Start the devcontainer
    """
    if not file().exists():
        print(color_message("No devcontainer settings found.  Run `invoke devcontainer.setup` first.", "red"))
        raise Exit(code=1)

    if not is_installed(ctx):
        print(color_message("Devcontainer CLI is not installed.  Run `invoke install-devcontainer-cli` first.", "red"))
        raise Exit(code=1)

    ctx.run(f"devcontainer up --workspace-folder {path}")


@task
def stop(ctx):
    """
    Stop the running devcontainer
    """
    if not file().exists():
        print(color_message("No devcontainer settings found. Run `inv devcontainer.setup` first and start it.", "red"))
        raise Exit(code=1)

    if not is_up(ctx):
        print(color_message("Devcontainer is not running.", "red"))
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


def is_installed(ctx) -> bool:
    res = ctx.run("which devcontainer", hide=True, warn=True)
    return res.ok


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
                    print(color_message("Generating the devcontainer file to run the linter in a container.", "orange"))
                    # TODO remove the hardcoded image and auto-pull it
                    setup(ctx, image=DEVCONTAINER_IMAGE)

                if not is_up(ctx):
                    print(color_message("Starting the devcontainer...", "orange"))
                    start(ctx)

                print(color_message("Running the command in the devcontainer...", "orange"))
                cli = DockerCLI(DEVCONTAINER_NAME)

                cmd = ["inv"] + sys.argv[1:]
                if not cli.run_command(cmd).ok:
                    print(color_message("Failed to run the command in the devcontainer.", "red"))
                    raise Exit(code=1)

                return

        func(ctx, *args, **kwargs)

    return _run_on_devcontainer
