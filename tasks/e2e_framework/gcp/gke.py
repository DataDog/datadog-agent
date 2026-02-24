import os

import yaml
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.e2e_framework import doc, tool
from tasks.e2e_framework.deploy import deploy
from tasks.e2e_framework.destroy import destroy

scenario_name = "gcp/gke"


@task(
    help={
        "install_agent": doc.install_agent,
        "agent_version": doc.container_agent_version,
        "stack_name": doc.stack_name,
        "agent_flavor": doc.agent_flavor,
        "helm_config": doc.helm_config,
        "local_chart_path": doc.local_chart_path,
        "kube_version": doc.kubernetes_version,
    }
)
def create_gke(
    ctx: Context,
    debug: bool | None = False,
    stack_name: str | None = None,
    install_agent: bool | None = True,
    install_workload: bool | None = True,
    agent_version: str | None = None,
    config_path: str | None = None,
    account: str | None = None,
    interactive: bool | None = True,
    full_image_path: str | None = None,
    cluster_agent_full_image_path: str | None = None,
    use_fakeintake: bool | None = False,
    use_autopilot: bool | None = False,
    agent_flavor: str | None = None,
    helm_config: str | None = None,
    local_chart_path: str | None = None,
    kube_version: str | None = None,
) -> None:
    """
    Create a new GKE environment.
    """
    from pydantic_core._pydantic_core import ValidationError

    from tasks.e2e_framework import config

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}") from e

    extra_flags = {
        "ddinfra:env": f"gcp/{account if account else cfg.get_gcp().account}",
        "ddinfra:gcp/defaultPublicKeyPath": cfg.get_gcp().publicKeyPath,
        "ddinfra:gcp/gke/enableAutopilot": use_autopilot,
        "ddagent:localChartPath": local_chart_path,
        "ddinfra:kubernetesVersion": kube_version,
    }

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
        config_path=config_path,
        full_image_path=full_image_path,
        cluster_agent_full_image_path=cluster_agent_full_image_path,
        use_fakeintake=use_fakeintake,
        agent_flavor=agent_flavor,
        helm_config=helm_config,
    )

    if interactive:
        tool.notify(ctx, "Your GKE cluster is now created")

    _show_connection_message(ctx, full_stack_name, interactive)


@task(help={"stack_name": doc.stack_name})
def destroy_gke(ctx: Context, stack_name: str | None = None, config_path: str | None = None):
    """
    Destroy a GKE environment created with invoke gcp.create-gke.
    """
    destroy(ctx, scenario_name=scenario_name, stack=stack_name, config_path=config_path)


def _show_connection_message(ctx: Context, full_stack_name: str, copy_to_clipboard: bool | None):
    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)
    kubeconfig_output = yaml.safe_load(outputs["dd-Cluster-gcp-gke"]["kubeConfig"])
    kubeconfig_content = yaml.dump(kubeconfig_output)
    kubeconfig = f"{full_stack_name}-config.yaml"
    f = os.open(path=kubeconfig, flags=(os.O_WRONLY | os.O_CREAT | os.O_TRUNC), mode=0o600)
    with open(f, "w") as file:
        file.write(kubeconfig_content)

    command = f"KUBECONFIG={kubeconfig} kubectl get nodes"

    print(f"\nYou can run the following command to connect to the GKE cluster\n\n{command}\n")
    if copy_to_clipboard:
        import pyperclip

        input("Press a key to copy command to clipboard...")
        pyperclip.copy(command)
