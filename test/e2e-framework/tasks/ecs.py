from typing import Optional

from invoke.context import Context
from invoke.tasks import task

from tasks.aws import doc as aws_doc

from . import doc


@task(
    help={
        "config_path": doc.config_path,
        "install_agent": doc.install_agent,
        "install_workload": doc.install_workload,
        "agent_version": doc.container_agent_version,
        "stack_name": doc.stack_name,
        "use_fargate": aws_doc.use_fargate,
        "linux_node_group": doc.linux_node_group,
        "linux_arm_node_group": doc.linux_arm_node_group,
        "bottlerocket_node_group": doc.bottlerocket_node_group,
        "windows_node_group": doc.windows_node_group,
    }
)
def create_ecs(
    ctx: Context,
    config_path: Optional[str] = None,
    stack_name: Optional[str] = None,
    install_agent: Optional[bool] = True,
    install_workload: Optional[bool] = True,
    agent_version: Optional[str] = None,
    use_fargate: bool = True,
    linux_node_group: bool = True,
    linux_arm_node_group: bool = False,
    bottlerocket_node_group: bool = True,
    windows_node_group: bool = False,
):
    print('This command is deprecated, please use `aws.create-ecs` instead')
    print("Running `aws.create-ecs`...")
    from tasks.aws.ecs import create_ecs as create_ecs_aws

    create_ecs_aws(
        ctx,
        config_path,
        stack_name,
        install_agent,
        install_workload,
        agent_version,
        use_fargate,
        linux_node_group,
        linux_arm_node_group,
        bottlerocket_node_group,
        windows_node_group,
    )


@task(help={"stack_name": doc.stack_name})
def destroy_ecs(ctx: Context, stack_name: Optional[str] = None):
    print('This command is deprecated, please use `aws.create-ecs` instead')
    print("Running `aws.create-ecs`...")
    from tasks.aws.ecs import destroy_ecs as destroy_ecs_aws

    destroy_ecs_aws(ctx, stack_name)
