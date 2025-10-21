import os
from typing import Optional

import pyperclip
import yaml
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task
from pydantic_core._pydantic_core import ValidationError

from tasks import config, doc, tool
from tasks.config import get_full_profile_path
from tasks.deploy import deploy
from tasks.destroy import destroy

scenario_name = "gcp/gke"


@task(
    help={
        "install_agent": doc.install_agent,
        "agent_version": doc.container_agent_version,
        "stack_name": doc.stack_name,
        "agent_flavor": doc.agent_flavor,
        "helm_config": doc.helm_config,
        "local_chart_path": doc.local_chart_path,
    }
)
def create_gke(
    ctx: Context,
    debug: Optional[bool] = False,
    stack_name: Optional[str] = None,
    install_agent: Optional[bool] = True,
    install_workload: Optional[bool] = True,
    agent_version: Optional[str] = None,
    config_path: Optional[str] = None,
    account: Optional[str] = None,
    interactive: Optional[bool] = True,
    full_image_path: Optional[str] = None,
    cluster_agent_full_image_path: Optional[str] = None,
    use_fakeintake: Optional[bool] = False,
    use_autopilot: Optional[bool] = False,
    agent_flavor: Optional[str] = None,
    helm_config: Optional[str] = None,
    local_chart_path: Optional[str] = None,
) -> None:
    """
    Create a new GKE environment.
    """

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {get_full_profile_path(config_path)}") from e

    extra_flags = {
        "ddinfra:env": f"gcp/{account if account else cfg.get_gcp().account}",
        "ddinfra:gcp/defaultPublicKeyPath": cfg.get_gcp().publicKeyPath,
        "ddinfra:gcp/gke/enableAutopilot": use_autopilot,
        "ddagent:localChartPath": local_chart_path,
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
def destroy_gke(ctx: Context, stack_name: Optional[str] = None, config_path: Optional[str] = None):
    """
    Destroy a GKE environment created with invoke gcp.create-gke.
    """
    destroy(ctx, scenario_name=scenario_name, stack=stack_name, config_path=config_path)


def _show_connection_message(ctx: Context, full_stack_name: str, copy_to_clipboard: Optional[bool]):
    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)
    kubeconfig_output = yaml.safe_load(outputs["dd-Cluster-gcp-gke"]["kubeConfig"])
    kubeconfig_content = yaml.dump(kubeconfig_output)
    kubeconfig = f"{full_stack_name}-config.yaml"
    f = os.open(path=kubeconfig, flags=(os.O_WRONLY | os.O_CREAT | os.O_TRUNC), mode=0o600)
    with open(f, "w") as f:
        f.write(kubeconfig_content)

    command = f"KUBECONFIG={kubeconfig} kubectl get nodes"

    print(f"\nYou can run the following command to connect to the GKE cluster\n\n{command}\n")
    if copy_to_clipboard:
        input("Press a key to copy command to clipboard...")
        pyperclip.copy(command)
