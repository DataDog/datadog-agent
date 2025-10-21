from typing import Optional

import pyperclip
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task
from pydantic import ValidationError

from tasks import config, doc, tool
from tasks.aws import doc as aws_doc
from tasks.aws.common import get_aws_wrapper
from tasks.aws.deploy import deploy
from tasks.destroy import destroy

scenario_name = "aws/ecs"


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
        "full_image_path": doc.full_image_path,
        "agent_flavor": doc.agent_flavor,
        "agent_env": doc.agent_env,
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
    full_image_path: Optional[str] = None,
    agent_flavor: Optional[str] = None,
    agent_env: Optional[str] = None,
):
    """
    Create a new ECS environment.
    """
    extra_flags = {
        "ddinfra:aws/ecs/fargateCapacityProvider": use_fargate,
        "ddinfra:aws/ecs/linuxECSOptimizedNodeGroup": linux_node_group,
        "ddinfra:aws/ecs/linuxECSOptimizedARMNodeGroup": linux_arm_node_group,
        "ddinfra:aws/ecs/linuxBottlerocketNodeGroup": bottlerocket_node_group,
        "ddinfra:aws/ecs/windowsLTSCNodeGroup": windows_node_group,
    }

    full_stack_name = deploy(
        ctx,
        scenario_name,
        config_path,
        stack_name=stack_name,
        install_agent=install_agent,
        install_workload=install_workload,
        agent_version=agent_version,
        extra_flags=extra_flags,
        full_image_path=full_image_path,
        agent_flavor=agent_flavor,
        agent_env=agent_env,
    )

    tool.notify(ctx, "Your ECS cluster is now created")

    _show_connection_message(ctx, config_path, full_stack_name)


def _show_connection_message(ctx: Context, config_path: Optional[str], full_stack_name: str):
    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)
    cluster_name = outputs["dd-Cluster-ecs"]["clusterName"]

    try:
        local_config = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}:{e}")

    command = f"{get_aws_wrapper(local_config.get_aws().get_account())} aws ecs list-tasks --cluster {cluster_name}"
    print(f"\nYou can run the following command to list tasks on the ECS cluster\n\n{command}\n")

    input("Press a key to copy command to clipboard...")
    pyperclip.copy(command)


@task(help={"stack_name": doc.stack_name})
def destroy_ecs(ctx: Context, stack_name: Optional[str] = None):
    """
    Destroy a ECS environment created with invoke aws.create-ecs.
    """
    destroy(ctx, scenario_name=scenario_name, stack=stack_name)
