from invoke.collection import Collection
from invoke.context import Context
from invoke.tasks import task

from tasks.e2e_framework import doc
from tasks.e2e_framework.aws.deploy import deploy
from tasks.e2e_framework.destroy import destroy

# scenario_name selects the Pulumi program registered under this key in
# test/e2e-framework/registry/scenarios.go ("aws/lustre": lustre.Run).
scenario_name = "aws/lustre"


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
        "pipeline_id": doc.pipeline_id,
        "install_agent": doc.install_agent,
        "agent_version": doc.agent_version,
        "debug": doc.debug,
        "use_fakeintake": doc.fakeintake,
        "agent_config_path": doc.agent_config_path,
        "local_package": doc.local_package,
    }
)
def create(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
    pipeline_id: str | None = None,
    install_agent: bool | None = True,
    agent_version: str | None = None,
    debug: bool | None = False,
    use_fakeintake: bool | None = False,
    agent_config_path: str | None = None,
    local_package: str | None = None,
) -> None:
    """
    Create the Lustre all-in-one Agent E2E lab.

    Provisions a single x86_64 EL9 EC2 host, bootstraps an all-in-one Lustre
    2.15 filesystem (MGS/MDS/OSS/client over loopback LNet) plus a continuous
    I/O + metadata workload, and installs the Datadog Agent with a three-instance
    `lustre.d` check config (client | mds | oss). All Lustre/workload resources
    are owned by this Pulumi stack; there is no external system to manage.
    """

    extra_flags: dict[str, object] = {}

    full_stack_name = deploy(
        ctx,
        scenario_name,
        config_path,
        stack_name=stack_name,
        pipeline_id=pipeline_id,
        install_agent=install_agent,
        agent_version=agent_version,
        debug=debug,
        extra_flags=extra_flags,
        use_fakeintake=use_fakeintake,
        agent_config_path=agent_config_path,
        local_package=local_package,
        needs_agent_containers=False,
    )

    print(f"Lustre lab created: {full_stack_name}")


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
    }
)
def destroy_lab(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
):
    """
    Destroy the Lustre all-in-one Agent E2E lab.

    Tears down only the resources created by this scenario's Pulumi stack
    (EC2 host + all-in-one Lustre filesystem + workload + Agent). Loop-backed
    Lustre targets live on the ephemeral instance, so no external state remains.
    """
    destroy(ctx, scenario_name=scenario_name, config_path=config_path, stack=stack_name)

    print("Lustre lab destroyed")


collection = Collection("lustre")
collection.add_task(create, name="create")
collection.add_task(destroy_lab, name="destroy")
