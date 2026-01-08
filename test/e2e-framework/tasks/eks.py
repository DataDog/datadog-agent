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
        "install_argorollout": doc.install_argorollout,
        "agent_version": doc.container_agent_version,
        "stack_name": doc.stack_name,
        "linux_node_group": doc.linux_node_group,
        "linux_arm_node_group": doc.linux_arm_node_group,
        "bottlerocket_node_group": doc.bottlerocket_node_group,
        "windows_node_group": doc.windows_node_group,
        "instance_type": aws_doc.instance_type,
    }
)
def create_eks(
    ctx: Context,
    config_path: Optional[str] = None,
    debug: Optional[bool] = False,
    stack_name: Optional[str] = None,
    install_agent: Optional[bool] = True,
    install_workload: Optional[bool] = True,
    install_argorollout: Optional[bool] = False,
    agent_version: Optional[str] = None,
    linux_node_group: bool = True,
    linux_arm_node_group: bool = False,
    bottlerocket_node_group: bool = True,
    windows_node_group: bool = False,
    instance_type: Optional[str] = None,
):
    print('This command is deprecated, please use `aws.create-eks` instead')
    print("Running `aws.create-eks`...")
    from tasks.aws.eks import create_eks as create_eks_aws

    create_eks_aws(
        ctx,
        config_path,
        debug,
        stack_name,
        install_agent,
        install_workload,
        install_argorollout,
        agent_version,
        linux_node_group,
        linux_arm_node_group,
        bottlerocket_node_group,
        windows_node_group,
        instance_type,
    )


@task(help={"stack_name": doc.stack_name})
def destroy_eks(ctx: Context, stack_name: Optional[str] = None):
    print('This command is deprecated, please use `aws.create-eks` instead')
    print("Running `aws.create-eks`...")
    from tasks.aws.eks import destroy_eks as destroy_eks_aws

    destroy_eks_aws(ctx, stack_name)
