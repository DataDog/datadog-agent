from invoke.context import Context
from invoke.tasks import task

from tasks.e2e_framework import doc, tool
from tasks.e2e_framework.aws import doc as aws_doc
from tasks.e2e_framework.aws.common import show_eks_connection_message
from tasks.e2e_framework.aws.deploy import deploy
from tasks.e2e_framework.destroy import destroy

scenario_name = "aws/benchmarkeks"


@task(
    help={
        "config_path": doc.config_path,
        "install_agent": doc.install_agent,
        "install_argorollout": doc.install_argorollout,
        "agent_version": doc.container_agent_version,
        "stack_name": doc.stack_name,
        "instance_type": aws_doc.instance_type,
        "full_image_path": doc.full_image_path,
        "cluster_agent_full_image_path": doc.cluster_agent_full_image_path,
        "baseline_version": "The container version of the baseline Agent",
        "baseline_full_image_path": "The full image path (registry:tag) of the baseline Agent image to deploy",
        "baseline_cluster_agent_version": "The container version of the baseline Cluster Agent",
        "baseline_cluster_agent_full_image_path": (
            "The full image path (registry:tag) of the baseline Cluster Agent image to deploy"
        ),
        "comparison_version": "The container version of the comparison Agent",
        "comparison_full_image_path": "The full image path (registry:tag) of the comparison Agent image to deploy",
        "comparison_cluster_agent_version": "The container version of the comparison Cluster Agent",
        "comparison_cluster_agent_full_image_path": (
            "The full image path (registry:tag) of the comparison Cluster Agent image to deploy"
        ),
        "agent_flavor": doc.agent_flavor,
        "helm_config": doc.helm_config,
        "local_chart_path": doc.local_chart_path,
        "kube_version": doc.kubernetes_version,
        "interactive": doc.interactive,
    }
)
def create_benchmarkeks(
    ctx: Context,
    config_path: str | None = None,
    debug: bool | None = False,
    stack_name: str | None = None,
    install_agent: bool | None = True,
    install_argorollout: bool | None = False,
    agent_version: str | None = None,
    instance_type: str | None = None,
    full_image_path: str | None = None,
    cluster_agent_full_image_path: str | None = None,
    baseline_version: str | None = None,
    baseline_full_image_path: str | None = None,
    baseline_cluster_agent_version: str | None = None,
    baseline_cluster_agent_full_image_path: str | None = None,
    comparison_version: str | None = None,
    comparison_full_image_path: str | None = None,
    comparison_cluster_agent_version: str | None = None,
    comparison_cluster_agent_full_image_path: str | None = None,
    agent_flavor: str | None = None,
    helm_config: str | None = None,
    local_chart_path: str | None = None,
    kube_version: str | None = None,
    interactive: bool | None = True,
):
    """
    Create a new EKS environment for benchmarking. It lasts around 20 minutes.

    This scenario deploys two independent Datadog Agent installations (baseline and comparison)
    in separate namespaces, pinned to dedicated node pools. What it compares is the resource
    footprint of the two Agent versions -- CPU, memory, goroutines, uploaded profiles -- while
    both monitor a strictly identical workload; the workload's own performance is not measured.
    Configure the two variants independently with the variant-specific version or full image
    path parameters.

    Example usage:
    - Compare two Agent versions:
      dda inv aws.create-benchmarkeks --baseline-version=7.55.0 --comparison-version=7.56.0

    - Compare specific image builds:
      dda inv aws.create-benchmarkeks \\
        --baseline-full-image-path=datadog/agent:7.55.0 \\
        --comparison-full-image-path=datadog/agent:7.56.0-rc.1

    A variant-specific full image path takes precedence over its version. If neither is
    provided for a variant, it falls back to the default full_image_path / agent_version
    parameters, so both variants would then be identical.
    """

    extra_flags = {
        "ddagent:localChartPath": local_chart_path,
        "ddtestworkload:deployArgoRollout": install_argorollout,
        "ddinfra:kubernetesVersion": kube_version,
        # Benchmarkeks-specific parameters for the baseline variant
        "ddagent:baselineVersion": baseline_version,
        "ddagent:baselineFullImagePath": baseline_full_image_path,
        "ddagent:baselineClusterAgentVersion": baseline_cluster_agent_version,
        "ddagent:baselineClusterAgentFullImagePath": baseline_cluster_agent_full_image_path,
        # Benchmarkeks-specific parameters for the comparison variant
        "ddagent:comparisonVersion": comparison_version,
        "ddagent:comparisonFullImagePath": comparison_full_image_path,
        "ddagent:comparisonClusterAgentVersion": comparison_cluster_agent_version,
        "ddagent:comparisonClusterAgentFullImagePath": comparison_cluster_agent_full_image_path,
    }

    # Override the instance type if specified
    if instance_type is not None:
        extra_flags["ddinfra:aws/defaultInstanceType"] = instance_type

    full_stack_name = deploy(
        ctx,
        scenario_name,
        debug=debug,
        app_key_required=True,
        stack_name=stack_name,
        install_agent=install_agent,
        # The benchmark workload is driven by the churn orchestrator, not by the
        # shared testing workload. Leaving the latter enabled would add an etcd
        # config provider pointing at an etcd this scenario never deploys, making
        # both Agents log connection errors for the whole benchmark run.
        install_workload=False,
        agent_version=agent_version,
        extra_flags=extra_flags,
        full_image_path=full_image_path,
        cluster_agent_full_image_path=cluster_agent_full_image_path,
        agent_flavor=agent_flavor,
        helm_config=helm_config,
    )

    if interactive:
        tool.notify(ctx, "Your benchmark EKS cluster is now created")

    show_eks_connection_message(ctx, full_stack_name, config_path, interactive)


@task(help={"stack_name": doc.stack_name})
def destroy_benchmarkeks(ctx: Context, stack_name: str | None = None):
    """
    Destroy a benchmark EKS environment created with invoke aws.create-benchmarkeks.
    """
    destroy(ctx, scenario_name=scenario_name, stack=stack_name)
