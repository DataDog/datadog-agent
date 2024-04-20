"""
vscode namespaced tags

Helpers for getting vscode set up nicely
"""

import json
import os
from typing import OrderedDict

from invoke import task

from tasks.build_tags import get_build_tags
from tasks.flavor import AgentFlavor
from tasks.libs.common.color import color_message

VSCODE_DIR = ".vscode"
VSCODE_FILE = "settings.json"


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

    use_tags = get_build_tags(
        build=target, arch=arch, flavor=flavor, build_include=build_include, build_exclude=build_exclude
    )

    if not os.path.exists(VSCODE_DIR):
        os.makedirs(VSCODE_DIR)

    settings = {}
    fullpath = os.path.join(VSCODE_DIR, VSCODE_FILE)
    if os.path.exists(fullpath):
        with open(fullpath) as sf:
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
    from tasks.devcontainer import setup

    print(color_message('This command is deprecated, please use `devcontainer.setup` instead', "orange"))
    print("Running `devcontainer.setup`...")
    setup(
        _,
        target=target,
        build_include=build_include,
        build_exclude=build_exclude,
        flavor=flavor,
        arch=arch,
        image=image,
    )
