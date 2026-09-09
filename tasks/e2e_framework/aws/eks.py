from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.e2e_framework import doc, tool
from tasks.e2e_framework.aws import doc as aws_doc
from tasks.e2e_framework.aws.common import show_eks_connection_message
from tasks.e2e_framework.aws.deploy import deploy
from tasks.e2e_framework.destroy import destroy

scenario_name = "aws/eks"


@task(
    help={
        "config_path": doc.config_path,
        "install_agent": doc.install_agent,
        "install_workload": doc.install_workload,
        "install_argorollout": doc.install_argorollout,
        "pipeline_id": doc.pipeline_id,
        "agent_version": doc.container_agent_version,
        "stack_name": doc.stack_name,
        "linux_node_group": doc.linux_node_group,
        "linux_arm_node_group": doc.linux_arm_node_group,
        "bottlerocket_node_group": doc.bottlerocket_node_group,
        "windows_node_group": doc.windows_node_group,
        "gpu_node_group": doc.gpu_node_group,
        "gpu_instance_type": doc.gpu_instance_type,
        "auto_mode": doc.eks_auto_mode,
        "instance_type": aws_doc.instance_type,
        "full_image_path": doc.full_image_path,
        "cluster_agent_full_image_path": doc.cluster_agent_full_image_path,
        "agent_flavor": doc.agent_flavor,
        "helm_config": doc.helm_config,
        "local_chart_path": doc.local_chart_path,
        "kube_version": doc.kubernetes_version,
        "interactive": doc.interactive,
    }
)
def create_eks(
    ctx: Context,
    config_path: str | None = None,
    debug: bool | None = False,
    stack_name: str | None = None,
    pipeline_id: str | None = None,
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
    auto_mode: bool | None = False,
    instance_type: str | None = None,
    full_image_path: str | None = None,
    cluster_agent_full_image_path: str | None = None,
    agent_flavor: str | None = None,
    helm_config: str | None = None,
    local_chart_path: str | None = None,
    kube_version: str | None = None,
    interactive: bool | None = True,
):
    """
    Create a new EKS environment. It lasts around 20 minutes.
    """

    # EKS Auto Mode provisions nodes itself, so managed node groups cannot coexist with
    # it. Reject the opt-in node groups rather than silently dropping them, which would
    # otherwise yield contradictory infrastructure (for example a Windows Agent with no
    # Windows nodes to run on). --linux-node-group and --bottlerocket-node-group default
    # to True, so an explicit request cannot be told apart from the default and they are
    # turned off rather than reported.
    if auto_mode:
        requested = [
            flag
            for flag, enabled in (
                ("--linux-arm-node-group", linux_arm_node_group),
                ("--windows-node-group", windows_node_group),
                ("--gpu-node-group", gpu_node_group),
            )
            if enabled
        ]
        if requested:
            raise Exit(
                f"--auto-mode is incompatible with managed node groups, remove {', '.join(requested)}: "
                "EKS Auto Mode provisions nodes itself."
            )
        linux_node_group = False
        bottlerocket_node_group = False

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
        "ddinfra:aws/eks/autoMode": auto_mode,
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
        pipeline_id=pipeline_id,
        install_agent=install_agent,
        install_workload=install_workload,
        agent_version=agent_version,
        extra_flags=extra_flags,
        full_image_path=full_image_path,
        cluster_agent_full_image_path=cluster_agent_full_image_path,
        agent_flavor=agent_flavor,
        helm_config=helm_config,
    )

    if interactive:
        tool.notify(ctx, "Your EKS cluster is now created")

    show_eks_connection_message(ctx, full_stack_name, config_path, interactive)


@task(help={"stack_name": doc.stack_name})
def destroy_eks(ctx: Context, stack_name: str | None = None):
    """
    Destroy a EKS environment created with invoke aws.create-eks.
    """
    destroy(ctx, scenario_name=scenario_name, stack=stack_name)
