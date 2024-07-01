"""
vscode namespaced tags

Helpers for getting vscode set up nicely
"""

from __future__ import annotations

import json
import os
import shutil
from collections import OrderedDict
from pathlib import Path

from invoke import Context, task
from invoke.exceptions import Exit

from tasks.build_tags import (
    build_tags,
    filter_incompatible_tags,
    get_build_tags,
    get_default_build_tags,
)
from tasks.flavor import AgentFlavor
from tasks.libs.common.color import Color, color_message
from tasks.libs.json import JSONWithCommentsDecoder

VSCODE_DIR = ".vscode"
VSCODE_FILE = "settings.json"
VSCODE_EXTENSIONS_FILE = "extensions.json"


@task(
    help={
        "targets": f"Comma separated list of targets to include. Possible values: all, {', '.join(build_tags[AgentFlavor.base].keys())}. Default: all",
        "flavor": f"Agent flavor to use. Possible values: {', '.join(AgentFlavor.__members__.keys())}. Default: {AgentFlavor.base.name}",
    }
)
def set_buildtags(
    _,
    targets="all",
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
):
    """
    Modifies vscode settings file for this project to include correct build tags
    """
    flavor = AgentFlavor[flavor]

    if targets == "all":
        targets = build_tags[flavor].keys()
    else:
        targets = targets.split(",")
        if not set(targets).issubset(build_tags[flavor]):
            print("Must choose valid targets. Valid targets are:")
            print(f'{", ".join(build_tags[flavor].keys())}')
            return

    if build_include is None:
        build_include = []
        for target in targets:
            build_include.extend(get_default_build_tags(build=target, flavor=flavor))
    else:
        build_include = filter_incompatible_tags(build_include.split(","))

    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    use_tags = get_build_tags(build_include, build_exclude)

    if not os.path.exists(VSCODE_DIR):
        os.makedirs(VSCODE_DIR)

    settings = {}
    fullpath = os.path.join(VSCODE_DIR, VSCODE_FILE)
    if os.path.exists(fullpath):
        with open(fullpath) as sf:
            settings = json.load(sf, object_pairs_hook=OrderedDict)

    settings["go.buildTags"] = ",".join(sorted(use_tags))

    with open(fullpath, "w") as sf:
        json.dump(settings, sf, indent=4, sort_keys=False, separators=(',', ': '))


@task
def setup_devcontainer(
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
    from tasks.devcontainer import setup

    print(color_message('This command is deprecated, please use `devcontainer.setup` instead', Color.ORANGE))
    print("Running `devcontainer.setup`...")
    setup(
        _,
        target=target,
        build_include=build_include,
        build_exclude=build_exclude,
        flavor=flavor,
        image=image,
    )


@task
def setup_extensions(ctx: Context):
    file = Path(VSCODE_DIR) / VSCODE_EXTENSIONS_FILE

    if not file.exists():
        print(color_message(f"The file {file} does not exist. Skipping installation of extensions.", Color.ORANGE))
        raise Exit(code=1)

    if shutil.which("code") is None:
        print(
            color_message(
                "`code` can't be found in your PATH. Skipping installation of extensions. See https://code.visualstudio.com/docs/setup/mac#_launching-from-the-command-line",
                Color.ORANGE,
            )
        )
        raise Exit(code=2)

    with open(file) as fd:
        content = json.load(fd, cls=JSONWithCommentsDecoder)

    for extension in content.get("recommendations", []):
        print(color_message(f"Installing extension {extension}", Color.BLUE))
        ctx.run(f"code --install-extension {extension} --force")
