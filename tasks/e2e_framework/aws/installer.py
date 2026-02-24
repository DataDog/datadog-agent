from invoke.context import Context
from invoke.tasks import task

from tasks.e2e_framework import doc
from tasks.e2e_framework.aws.deploy import deploy
from tasks.e2e_framework.destroy import destroy

scenario_name = "aws/installer"


@task(
    help={
        "debug": doc.debug,
        "pipeline_id": doc.pipeline_id,
        "site": doc.site,
        "agent_flavor": doc.agent_flavor,
    }
)
def create_installer_lab(
    ctx: Context,
    debug: bool | None = False,
    pipeline_id: str | None = None,
    site: str | None = "datad0g.com",
    agent_flavor: str | None = None,
):
    full_stack_name = deploy(
        ctx,
        scenario_name,
        stack_name="installer-lab",
        pipeline_id=pipeline_id,
        install_installer=True,
        debug=debug,
        extra_flags={"ddagent:site": site},
        agent_flavor=agent_flavor,
    )

    print(f"Installer lab created: {full_stack_name}")


@task
def destroy_installer_lab(
    ctx: Context,
):
    destroy(ctx, scenario_name=scenario_name, stack="installer-lab")

    print("Installer lab destroyed")
