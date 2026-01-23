import os

import yaml
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.e2e_framework import doc, tool
from tasks.e2e_framework.deploy import deploy
from tasks.e2e_framework.destroy import destroy

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
    agent_flavor: str | None = None,
    helm_config: str | None = None,
    local_chart_path: str | None = None,
):
    """
    Create a new AKS environment. It lasts around 5 minutes.
    """

    from pydantic_core._pydantic_core import ValidationError

    from tasks.e2e_framework import config

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}") from e

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
def destroy_aks(ctx: Context, stack_name: str | None = None, config_path: str | None = None):
    """
    Destroy a AKS environment created with invoke az.create-aks.
    """
    destroy(ctx, scenario_name=scenario_name, stack=stack_name, config_path=config_path)


def _show_connection_message(ctx: Context, full_stack_name: str, copy_to_clipboard: bool | None):
    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)
    kubeconfig_output = yaml.safe_load(outputs["dd-Cluster-az-aks"]["kubeConfig"])
    kubeconfig_content = yaml.dump(kubeconfig_output)
    kubeconfig = f"{full_stack_name}-config.yaml"
    f = os.open(path=kubeconfig, flags=(os.O_WRONLY | os.O_CREAT | os.O_TRUNC), mode=0o600)
    with open(f, "w") as file:
        file.write(kubeconfig_content)

    command = f"KUBECONFIG={kubeconfig} kubectl get nodes"

    print(f"\nYou can run the following command to connect to the AKS cluster\n\n{command}\n")
    if copy_to_clipboard:
        import pyperclip

        input("Press a key to copy command to clipboard...")
        pyperclip.copy(command)
