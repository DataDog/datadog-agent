"""
vscode namespaced tags

Helpers for getting vscode set up nicely
"""
import json
import os
from collections import OrderedDict

from invoke import task

from .build_tags import build_tags, filter_incompatible_tags, get_build_tags, get_default_build_tags
from .flavor import AgentFlavor

VSCODE_DIR = ".vscode"
VSCODE_FILE = "settings.json"
VSCODE_DEVCONTAINER_DIR = ".devcontainer"
VSCODE_DEVCONTAINER_FILE = "devcontainer.json"


@task
def set_buildtags(
    _,
    target="agent",
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
    arch='x64',
):
    """
    Modifies vscode settings file for this project to include correct build tags
    """
    flavor = AgentFlavor[flavor]

    if target not in build_tags[flavor]:
        print("Must choose a valid target.  Valid targets are: \n")
        print(f'{", ".join(build_tags[flavor].keys())} \n')
        return

    build_include = (
        get_default_build_tags(build=target, arch=arch, flavor=flavor)
        if build_include is None
        else filter_incompatible_tags(build_include.split(","), arch=arch)
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    use_tags = get_build_tags(build_include, build_exclude)

    if not os.path.exists(VSCODE_DIR):
        os.makedirs(VSCODE_DIR)

    settings = {}
    fullpath = os.path.join(VSCODE_DIR, VSCODE_FILE)
    if os.path.exists(fullpath):
        with open(fullpath, "r") as sf:
            settings = json.load(sf, object_pairs_hook=OrderedDict)

    settings["go.buildTags"] = ",".join(use_tags)

    with open(fullpath, "w") as sf:
        json.dump(settings, sf, indent=4, sort_keys=False, separators=(',', ': '))


@task
def setup_devcontainer(
    _,
    target="agent",
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
    arch='x64',
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
        get_default_build_tags(build=target, arch=arch, flavor=flavor)
        if build_include is None
        else filter_incompatible_tags(build_include.split(","), arch=arch)
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    use_tags = get_build_tags(build_include, build_exclude)

    if not os.path.exists(VSCODE_DEVCONTAINER_DIR):
        os.makedirs(VSCODE_DEVCONTAINER_DIR)

    devcontainer = {}
    fullpath = os.path.join(VSCODE_DEVCONTAINER_DIR, VSCODE_DEVCONTAINER_FILE)
    if os.path.exists(fullpath):
        with open(fullpath, "r") as sf:
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
    devcontainer["runArgs"] = ["--cap-add=SYS_PTRACE", "--security-opt", "seccomp=unconfined"]
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
                    "/workspaces/datadog-agent/.golangci.yml",
                ],
                "[go]": {
                    "editor.formatOnSave": True,
                },
                "gopls": {"formatting.local": "github.com/DataDog/datadog-agent"},
            },
            "extensions": ["golang.Go"],
        }
    }
    devcontainer[
        "postStartCommand"
    ] = "git config --global --add safe.directory /workspaces/datadog-agent && invoke install-tools && invoke deps"

    with open(fullpath, "w") as sf:
        json.dump(devcontainer, sf, indent=4, sort_keys=False, separators=(',', ': '))
