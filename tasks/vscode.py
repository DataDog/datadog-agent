"""
vscode namespaced tags

Helpers for getting vscode set up nicely
"""
from invoke import task
from libs.common.color import color_message

from tasks.flavor import AgentFlavor


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
    from tasks.devcontainer import set_buildtags

    print(color_message('This command is deprecated, please use `devcontainer.set_buildtags` instead', "orange"))
    print("Running `devcontainer.set_buildtags`...")
    set_buildtags(
        _,
        target=target,
        build_include=build_include,
        build_exclude=build_exclude,
        flavor=flavor,
        arch=arch,
    )


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
    print("Running `devcontainer.set_buildtags`...")
    setup(
        _,
        target=target,
        build_include=build_include,
        build_exclude=build_exclude,
        flavor=flavor,
        arch=arch,
        image=image,
    )
