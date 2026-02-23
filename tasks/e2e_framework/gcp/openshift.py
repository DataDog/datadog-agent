from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.e2e_framework import doc
from tasks.e2e_framework.deploy import deploy
from tasks.e2e_framework.destroy import destroy

scenario_name = "gcp/openshiftvm"


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
        "pull_secret_path": doc.pull_secret_path,
        "install_agent": doc.install_agent,
        "install_workload": doc.install_workload,
        "use_fakeintake": doc.fakeintake,
        "use_loadBalancer": doc.use_loadBalancer,
        "agent_version": doc.agent_version,
        "full_image_path": doc.full_image_path,
        "cluster_agent_full_image_path": doc.cluster_agent_full_image_path,
        "agent_flavor": doc.agent_flavor,
        "helm_config": doc.helm_config,
    }
)
def create_openshift(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
    pull_secret_path: str | None = None,
    use_nested_virtualization: bool | None = True,
    install_agent: bool | None = True,
    install_workload: bool | None = True,
    use_fakeintake: bool | None = False,
    use_loadBalancer: bool | None = False,
    agent_version: str | None = None,
    full_image_path: str | None = None,
    cluster_agent_full_image_path: str | None = None,
    agent_flavor: str | None = None,
    helm_config: str | None = None,
):
    """
    Create an OpenShift environment.
    """

    from pydantic_core._pydantic_core import ValidationError

    from tasks.e2e_framework import config

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}") from e

    # Use parameter if provided during invoke setup, otherwise use config
    if not pull_secret_path:
        pull_secret_path = cfg.get_gcp().pullSecretPath
        if not pull_secret_path:
            raise Exit(
                "pull_secret_path is required. Either use invoke.gcp.create-openshift -p <pull_secret_path> or configure it with 'invoke setup'"
            )

    extra_flags = {
        "scenario": scenario_name,
        "ddinfra:env": f"gcp/{cfg.get_gcp().account}",
        "ddinfra:gcp/defaultPublicKeyPath": cfg.get_gcp().publicKeyPath,
        "ddinfra:gcp/openshift/pullSecretPath": pull_secret_path,
        "ddinfra:gcp/enableNestedVirtualization": use_nested_virtualization,
        "ddinfra:gcp/defaultInstanceType": "n2-standard-8",
        "ddinfra:gcp/fakeintakeWithLB": use_loadBalancer,
    }

    deploy(
        ctx,
        scenario_name,
        config_path,
        stack_name=stack_name,
        install_agent=install_agent,
        install_workload=install_workload,
        use_fakeintake=use_fakeintake,
        agent_version=agent_version,
        extra_flags=extra_flags,
        app_key_required=True,
        full_image_path=full_image_path,
        cluster_agent_full_image_path=cluster_agent_full_image_path,
        agent_flavor=agent_flavor,
        helm_config=helm_config,
    )


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
    }
)
def destroy_openshift(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
):
    """
    Destroy an environment created by invoke gcp.create-openshift.
    """
    destroy(
        ctx,
        scenario_name=scenario_name,
        config_path=config_path,
        stack=stack_name,
    )
