from invoke.context import Context
from invoke.tasks import task

from tasks.e2e_framework import config, doc, tool
from tasks.e2e_framework.aws import doc as aws_doc
from tasks.e2e_framework.aws.common import get_architectures, get_default_architecture
from tasks.e2e_framework.aws.deploy import deploy
from tasks.e2e_framework.destroy import destroy

scenario_name = "aws/milvus"


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
def create_milvus(
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
    Create an AWS Docker environment running Milvus, a Milvus load generator,
    and a Datadog Agent configured with the Milvus integration.
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
        tool.notify(ctx, "Your Milvus environment is now created")

    _show_connection_message(ctx, full_stack_name, interactive, config_path)


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
    }
)
def destroy_milvus(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
):
    """
    Destroy an environment created by invoke aws.create-milvus.
    """
    destroy(
        ctx,
        scenario_name=scenario_name,
        config_path=config_path,
        stack=stack_name,
    )


def _show_connection_message(
    ctx: Context, full_stack_name: str, copy_to_clipboard: bool | None, config_path: str | None = None
):
    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)
    remote_host = tool.RemoteHost("aws-milvus", outputs)
    host = remote_host.address
    user = remote_host.user

    cfg = config.get_local_config(config_path)
    private_key_path = cfg.get_aws().privateKeyPath
    ssh_identity = f" -i {private_key_path}" if private_key_path else ""

    command = (
        f"\nssh{ssh_identity} {user}@{host} -- 'docker ps --filter name=milvus --filter name=datadog-agent'\n"
        + f'docker context create pulumi-{host} --docker "host=ssh://{user}@{host}"\n'
        + f"docker --context pulumi-{host} logs milvus-load --tail 50\n"
        + f"docker --context pulumi-{host} exec datadog-agent agent status | grep -A20 -i milvus\n"
    )
    print(f"Useful commands for your Milvus lab:\n\n{command}")

    if copy_to_clipboard:
        import pyperclip

        input("Press a key to copy command to clipboard...")
        pyperclip.copy(command)


def _get_architecture(architecture: str | None) -> str:
    architectures = get_architectures()
    if architecture is None:
        architecture = get_default_architecture()
    if architecture.lower() not in architectures:
        from invoke.exceptions import Exit

        raise Exit(
            f"The architecture '{architecture}' is not supported. Possible values are {', '.join(architectures)}"
        )
    return architecture
