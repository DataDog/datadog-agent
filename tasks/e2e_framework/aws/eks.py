import json
import os

import yaml
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.e2e_framework import doc, tool
from tasks.e2e_framework.aws import doc as aws_doc
from tasks.e2e_framework.aws.common import get_aws_wrapper
from tasks.e2e_framework.aws.deploy import deploy
from tasks.e2e_framework.destroy import destroy

scenario_name = "aws/eks"


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
        "gpu_node_group": doc.gpu_node_group,
        "gpu_instance_type": doc.gpu_instance_type,
        "instance_type": aws_doc.instance_type,
        "full_image_path": doc.full_image_path,
        "cluster_agent_full_image_path": doc.cluster_agent_full_image_path,
        "agent_flavor": doc.agent_flavor,
        "helm_config": doc.helm_config,
        "local_chart_path": doc.local_chart_path,
        "kube_version": doc.kubernetes_version,
    }
)
def create_eks(
    ctx: Context,
    config_path: str | None = None,
    debug: bool | None = False,
    stack_name: str | None = None,
    install_agent: bool | None = True,
    install_workload: bool | None = True,
    install_argorollout: bool | None = False,
    agent_version: str | None = None,
    linux_node_group: bool = True,
    linux_arm_node_group: bool = False,
    bottlerocket_node_group: bool = True,
    windows_node_group: bool = False,
    gpu_node_group: bool = False,
    gpu_instance_type: str | None = None,
    instance_type: str | None = None,
    full_image_path: str | None = None,
    cluster_agent_full_image_path: str | None = None,
    agent_flavor: str | None = None,
    helm_config: str | None = None,
    local_chart_path: str | None = None,
    kube_version: str | None = None,
):
    """
    Create a new EKS environment. It lasts around 20 minutes.
    """

    # When GPU node group is enabled, disable other node groups for a GPU-only cluster
    # GPU instances are x86_64 only, so ARM is incompatible
    if gpu_node_group:
        linux_node_group = False
        linux_arm_node_group = False
        bottlerocket_node_group = False

    extra_flags = {
        "ddinfra:aws/eks/linuxARMNodeGroup": linux_arm_node_group,
        "ddinfra:aws/eks/linuxBottlerocketNodeGroup": bottlerocket_node_group,
        "ddinfra:aws/eks/linuxNodeGroup": str(linux_node_group),
        "ddinfra:aws/eks/windowsNodeGroup": windows_node_group,
        "ddinfra:aws/eks/gpuNodeGroup": gpu_node_group,
        "ddinfra:aws/eks/gpuInstanceType": gpu_instance_type if gpu_instance_type else "g4dn.xlarge",
        "ddagent:localChartPath": local_chart_path,
        "ddtestworkload:deployArgoRollout": install_argorollout,
        "ddinfra:kubernetesVersion": kube_version,
    }

    # Override the instance type if specified
    # ARM node groups use defaultARMInstanceType, all others (Linux, Bottlerocket, Windows) use defaultInstanceType
    if instance_type is not None:
        if linux_arm_node_group:
            extra_flags["ddinfra:aws/defaultARMInstanceType"] = instance_type
        else:
            extra_flags["ddinfra:aws/defaultInstanceType"] = instance_type

    full_stack_name = deploy(
        ctx,
        scenario_name,
        debug=debug,
        app_key_required=True,
        stack_name=stack_name,
        install_agent=install_agent,
        install_workload=install_workload,
        agent_version=agent_version,
        extra_flags=extra_flags,
        full_image_path=full_image_path,
        cluster_agent_full_image_path=cluster_agent_full_image_path,
        agent_flavor=agent_flavor,
        helm_config=helm_config,
    )

    tool.notify(ctx, "Your EKS cluster is now created")

    _show_connection_message(ctx, full_stack_name, config_path)


def _show_connection_message(ctx: Context, full_stack_name: str, config_path: str | None):
    import pyperclip
    from pydantic import ValidationError

    from tasks.e2e_framework import config

    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)
    kubeconfig_output = json.loads(outputs["dd-Cluster-eks"]["kubeConfig"])
    kubeconfig_content = yaml.dump(kubeconfig_output)
    kubeconfig = f"{full_stack_name}-kubeconfig.yaml"
    f = os.open(path=kubeconfig, flags=(os.O_WRONLY | os.O_CREAT | os.O_TRUNC), mode=0o600)
    with open(f, "w") as file:
        file.write(kubeconfig_content)

    try:
        local_config = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}:{e}") from e

    command = f"KUBECONFIG={kubeconfig} {get_aws_wrapper(local_config.get_aws().get_account())} kubectl get nodes"

    print(f"\nYou can run the following command to connect to the EKS cluster\n\n{command}\n")

    input("Press a key to copy command to clipboard...")
    pyperclip.copy(command)


@task(help={"stack_name": doc.stack_name})
def destroy_eks(ctx: Context, stack_name: str | None = None):
    """
    Destroy a EKS environment created with invoke aws.create-eks.
    """
    destroy(ctx, scenario_name=scenario_name, stack=stack_name)
