"""
Packer namespaced tasks
"""

from invoke import task
from invoke.exceptions import Exit

DEFAULT_BUILDERS = [
    "parallels-iso",
    "vmware-iso",
    "virtualbox-iso",
]


@task
def build(ctx, os="windows-10", provider="virtualbox-iso"):
    """
    Build the Vagrant box

    Example invokation:
        inv packer.build --os=windows-10
    """

    if provider not in DEFAULT_BUILDERS:
        print("Error: unknown provider")
        return Exit(code=1)

    command = "ruby -r \"./gen-packer.rb\" -e \"build('{name}', '{type}')\" > packer.json"

    if os == "windows-10":
        ctx.run(command.format(name="windows_10_ent", type="client"))
    elif os == "windows-server":
        ctx.run(command.format(name="windows_2019_core", type="server"))
    else:
        print("Error: unknown OS")
        raise Exit(code=1)

    ctx.run("packer build --only=" + provider + " packer.json")
