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

scenario_name = "az/aks"


@task(
    help={
        "install_agent": doc.install_agent,
        "install_workload": doc.install_workload,
        "agent_version": doc.container_agent_version,
        "stack_name": doc.stack_name,
        "agent_flavor": doc.agent_flavor,
        "helm_config": doc.helm_config,
        "local_chart_path": doc.local_chart_path,
    }
)
def create_aks(
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
    agent_flavor: Optional[str] = None,
    helm_config: Optional[str] = None,
    local_chart_path: Optional[str] = None,
):
    """
    Create a new AKS environment. It lasts around 5 minutes.
    """

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {get_full_profile_path(config_path)}") from e

    extra_flags = {
        "ddinfra:env": f"az/{account if account else cfg.get_azure().account}",
        "ddinfra:az/defaultPublicKeyPath": cfg.get_azure().publicKeyPath,
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
        tool.notify(ctx, "Your AKS cluster is now created")

    _show_connection_message(ctx, full_stack_name, interactive)


@task(help={"stack_name": doc.stack_name})
def destroy_aks(ctx: Context, stack_name: Optional[str] = None, config_path: Optional[str] = None):
    """
    Destroy a AKS environment created with invoke az.create-aks.
    """
    destroy(ctx, scenario_name=scenario_name, stack=stack_name, config_path=config_path)


def _show_connection_message(ctx: Context, full_stack_name: str, copy_to_clipboard: Optional[bool]):
    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)
    kubeconfig_output = yaml.safe_load(outputs["dd-Cluster-az-aks"]["kubeConfig"])
    kubeconfig_content = yaml.dump(kubeconfig_output)
    kubeconfig = f"{full_stack_name}-config.yaml"
    f = os.open(path=kubeconfig, flags=(os.O_WRONLY | os.O_CREAT | os.O_TRUNC), mode=0o600)
    with open(f, "w") as f:
        f.write(kubeconfig_content)

    command = f"KUBECONFIG={kubeconfig} kubectl get nodes"

    print(f"\nYou can run the following command to connect to the AKS cluster\n\n{command}\n")
    if copy_to_clipboard:
        input("Press a key to copy command to clipboard...")
        pyperclip.copy(command)
