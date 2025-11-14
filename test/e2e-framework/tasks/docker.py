from typing import Optional

from invoke.context import Context
from invoke.tasks import task

from tasks.aws import doc as aws_doc

from . import doc


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
    }
)
def create_docker(
    ctx: Context,
    config_path: Optional[str] = None,
    stack_name: Optional[str] = None,
    install_agent: Optional[bool] = True,
    agent_version: Optional[str] = None,
    architecture: Optional[str] = None,
    use_fakeintake: Optional[bool] = False,
    use_loadBalancer: Optional[bool] = False,
    interactive: Optional[bool] = True,
):
    print('This command is deprecated, please use `aws.create-docker` instead')
    print("Running `aws.create-docker`...")
    from tasks.aws.docker import create_docker as create_docker_aws

    create_docker_aws(
        ctx,
        config_path,
        stack_name,
        install_agent,
        agent_version,
        architecture,
        use_fakeintake,
        use_loadBalancer,
        interactive,
    )


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
    }
)
def destroy_docker(
    ctx: Context,
    config_path: Optional[str] = None,
    stack_name: Optional[str] = None,
):
    print('This command is deprecated, please use `aws.destroy-docker` instead')
    print("Running `aws.destroy-docker`...")
    from tasks.aws.docker import destroy_docker as destroy_docker_aws

    destroy_docker_aws(ctx, config_path, stack_name)
