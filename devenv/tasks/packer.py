"""
Agent namespaced tasks
"""
from __future__ import print_function
import os

import invoke
from invoke import task
from invoke.exceptions import Exit, ParseError

@task
def build(ctx, os):
    """
    Build the Vagrant box
    
    Example invokation:
        inv agent.build --os=windows-10
    """

    print(os)

    command = "ruby -r \"./gen-packer.rb\" -e \"build('{name}', '{type}')\" > packer.json"

    if os == "windows-10":
        ctx.run(command.format(
            name="windows_10_ent",
            type="client"
        ))
    elif os == "windows-server":
        ctx.run(command.format(
            name="windows_2019_core",
            type="server"
        ))
    else:
        print("Error: unknown OS")
        raise Exit(code=1)

    ctx.run("packer build packer.json")
