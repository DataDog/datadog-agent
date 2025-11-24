from typing import Optional

from invoke.context import Context
from invoke.tasks import task

from tasks import doc
from tasks.aws.deploy import deploy
from tasks.destroy import destroy

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
    debug: Optional[bool] = False,
    pipeline_id: Optional[str] = None,
    site: Optional[str] = "datad0g.com",
    agent_flavor: Optional[str] = None,
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
