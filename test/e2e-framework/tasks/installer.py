from typing import Optional

from invoke.context import Context
from invoke.tasks import task

from . import doc


@task(
    help={
        "debug": doc.debug,
        "pipeline_id": doc.pipeline_id,
        "site": doc.site,
    }
)
def create_installer_lab(
    ctx: Context,
    debug: Optional[bool] = False,
    pipeline_id: Optional[str] = None,
    site: Optional[str] = "datad0g.com",
):
    print('This command is deprecated, please use `aws.create-installer-lab` instead')
    print("Running `aws.create-installer-lab`...")
    from tasks.aws.installer import create_installer_lab as create_installer_lab_aws

    create_installer_lab_aws(
        ctx,
        debug,
        pipeline_id,
        site,
    )


@task
def destroy_installer_lab(
    ctx: Context,
):
    print('This command is deprecated, please use `aws.destroy-installer-lab` instead')
    print("Running `aws.destroy-installer-lab`...")
    from tasks.aws.installer import destroy_installer_lab as destroy_installer_lab_aws

    destroy_installer_lab_aws(ctx)
