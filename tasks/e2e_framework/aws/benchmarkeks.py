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
        "baseline_version": doc.baseline_version,
        "baseline_full_image_path": doc.baseline_full_image_path,
        "baseline_cluster_agent_version": doc.baseline_cluster_agent_version,
        "baseline_cluster_agent_full_image_path": doc.baseline_cluster_agent_full_image_path,
        "comparison_version": doc.comparison_version,
        "comparison_full_image_path": doc.comparison_full_image_path,
        "comparison_cluster_agent_version": doc.comparison_cluster_agent_version,
        "comparison_cluster_agent_full_image_path": doc.comparison_cluster_agent_full_image_path,
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
    in separate namespaces, pinned to dedicated node pools, to enable performance comparisons of
    a strictly identical workload. Configure the two variants independently with the
    variant-specific version or full image path parameters.

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
        agent_version=agent_version,
        extra_flags=extra_flags,
        full_image_path=full_image_path,
        cluster_agent_full_image_path=cluster_agent_full_image_path,
        agent_flavor=agent_flavor,
        helm_config=helm_config,
    )

    if interactive:
        tool.notify(ctx, "Your benchmark EKS cluster is now created")

    _show_connection_message(ctx, full_stack_name, config_path, interactive)


def _show_connection_message(
    ctx: Context, full_stack_name: str, config_path: str | None, interactive: bool | None = True
):
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

    print(f"\nYou can run the following command to connect to the benchmark EKS cluster\n\n{command}\n")

    if interactive:
        import pyperclip

        input("Press a key to copy command to clipboard...")
        pyperclip.copy(command)


@task(help={"stack_name": doc.stack_name})
def destroy_benchmarkeks(ctx: Context, stack_name: str | None = None):
    """
    Destroy a benchmark EKS environment created with invoke aws.create-benchmarkeks.
    """
    destroy(ctx, scenario_name=scenario_name, stack=stack_name)
