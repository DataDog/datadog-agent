from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.e2e_framework import doc, tool
from tasks.e2e_framework.aws import doc as aws_doc
from tasks.e2e_framework.aws.common import get_architectures, get_default_architecture
from tasks.e2e_framework.aws.deploy import deploy
from tasks.e2e_framework.destroy import destroy

scenario_name = "aws/dockervm"


@task(
    help={
        "config_path": doc.config_path,
        "install_agent": doc.install_agent,
        "agent_version": doc.container_agent_version,
        "stack_name": doc.stack_name,
        "architecture": aws_doc.architecture,
        "use_fakeintake": doc.fakeintake,
        "use_loadBalancer": doc.use_loadBalancer,
        "interactive": doc.interactive,
        "full_image_path": doc.full_image_path,
        "agent_flavor": doc.agent_flavor,
        "agent_env": doc.agent_env,
    },
)
def create_docker(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
    install_agent: bool | None = True,
    agent_version: str | None = None,
    architecture: str | None = None,
    use_fakeintake: bool | None = False,
    use_loadBalancer: bool | None = False,
    interactive: bool | None = True,
    full_image_path: str | None = None,
    agent_flavor: str | None = None,
    agent_env: str | None = None,
):
    """
    Create a docker environment.
    """

    extra_flags = {
        "ddinfra:osDescriptor": f"::{_get_architecture(architecture)}",
        "ddinfra:deployFakeintakeWithLoadBalancer": use_loadBalancer,
    }

    full_stack_name = deploy(
        ctx,
        scenario_name,
        config_path,
        key_pair_required=True,
        stack_name=stack_name,
        install_agent=install_agent,
        agent_version=agent_version,
        use_fakeintake=use_fakeintake,
        extra_flags=extra_flags,
        full_image_path=full_image_path,
        agent_flavor=agent_flavor,
        agent_env=agent_env,
    )

    if interactive:
        tool.notify(ctx, "Your Docker environment is now created")

    _show_connection_message(ctx, full_stack_name, interactive)


def _show_connection_message(ctx: Context, full_stack_name: str, copy_to_clipboard: bool | None):
    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)
    remoteHost = tool.RemoteHost("aws-vm", outputs)
    host = remoteHost.address
    user = remoteHost.user

    command = (
        f"\nssh {user}@{host} --  'echo \"Successfully connected to VM\" && exit' \n"
        + f'docker context create pulumi-{host} --docker "host=ssh://{user}@{host}"\n'
        + f"docker --context pulumi-{host} container ls\n"
    )
    print(f"If you want to use docker context, you can run the following commands \n\n{command}")

    if copy_to_clipboard:
        import pyperclip

        input("Press a key to copy command to clipboard...")
        pyperclip.copy(command)


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
    }
)
def destroy_docker(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
):
    """
    Destroy an environment created by invoke aws.create-docker.
    """
    destroy(
        ctx,
        scenario_name=scenario_name,
        config_path=config_path,
        stack=stack_name,
    )


def _get_architecture(architecture: str | None) -> str:
    architectures = get_architectures()
    if architecture is None:
        architecture = get_default_architecture()
    if architecture.lower() not in architectures:
        raise Exit(f"The os family '{architecture}' is not supported. Possibles values are {', '.join(architectures)}")
    return architecture
