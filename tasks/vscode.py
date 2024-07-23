"""
vscode namespaced tags

Helpers for getting vscode set up nicely
"""

from __future__ import annotations

import json
import os
import shutil
import sys
from collections import OrderedDict
from pathlib import Path

from invoke import Context, task
from invoke.exceptions import Exit

from tasks.build_tags import build_tags, compute_config_build_tags
from tasks.flavor import AgentFlavor
from tasks.libs.common.color import Color, color_message
from tasks.libs.json import JSONWithCommentsDecoder

VSCODE_DIR = ".vscode"
VSCODE_SETTINGS_FILE = "settings.json"
VSCODE_SETTINGS_TEMPLATE = "settings.json.template"
VSCODE_TASKS_FILE = "tasks.json"
VSCODE_TASKS_TEMPLATE = "tasks.json.template"
VSCODE_EXTENSIONS_FILE = "extensions.json"


@task
def setup(ctx, force=False):
    """
    Set up vscode for this project

    - force: If True, will override the existing settings
    """
    print(color_message("* Setting up extensions", Color.BOLD))
    setup_extensions(ctx)
    print(color_message("* Setting up tasks", Color.BOLD))
    setup_tasks(ctx, force)
    print(color_message("* Setting up settings", Color.BOLD))
    setup_settings(ctx, force)
    # TODO: setup_launch (see #27508)


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
    use_tags = compute_config_build_tags(
        targets=targets,
        build_include=build_include,
        build_exclude=build_exclude,
        flavor=flavor,
    )

    if not os.path.exists(VSCODE_DIR):
        os.makedirs(VSCODE_DIR)

    settings = {}
    fullpath = os.path.join(VSCODE_DIR, VSCODE_SETTINGS_FILE)
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
    from tasks import devcontainer

    print(color_message('This command is deprecated, please use `devcontainer.setup` instead', Color.ORANGE))
    print("Running `devcontainer.setup`...")
    devcontainer.setup(
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


@task
def setup_tasks(_, force=False):
    """
    Creates the initial .vscode/tasks.json file based on the template

    - force: If True, will override the existing tasks file
    """
    tasks = Path(VSCODE_DIR) / VSCODE_TASKS_FILE
    template = Path(VSCODE_DIR) / VSCODE_TASKS_TEMPLATE

    print(color_message("Creating initial VSCode tasks file...", Color.BLUE))
    if tasks.exists():
        message = 'overriding current file' if force else 'skipping...'
        print(color_message("warning:", Color.ORANGE), 'VSCode tasks file already exists,', message)
        if not force:
            return

    shutil.copy(template, tasks)
    print(color_message("VSCode tasks file created successfully.", Color.GREEN))


@task
def setup_settings(_, force=False):
    """
    Creates the initial .vscode/settings.json file

    - force: If True, will override the existing settings file
    """
    settings = Path(VSCODE_DIR) / VSCODE_SETTINGS_FILE
    template = Path(VSCODE_DIR) / VSCODE_SETTINGS_TEMPLATE

    print(color_message("Creating initial VSCode setting file...", Color.BLUE))
    if settings.exists():
        message = 'overriding current file' if force else 'skipping...'
        print(color_message("warning:", Color.ORANGE), 'VSCode settings file already exists,', message)
        if not force:
            return

    build_tags = sorted(compute_config_build_tags())
    with open(template) as template_f, open(settings, "w") as settings_f:
        vscode_config_template = template_f.read()
        settings_f.write(
            vscode_config_template.format(
                build_tags=",".join(build_tags),
                workspace_folder=os.getcwd(),
                excluded_directories=["-rtloader/test", "-test/benchmarks", "-test/integration"]
                if sys.platform != "linux"
                else [],
            ).replace("'", '"')
        )
    print(color_message("VSCode settings file created successfully.", Color.GREEN))
