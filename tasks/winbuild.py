from invoke.tasks import task

from tasks.flavor import AgentFlavor
from tasks.msi import build as agent_msi_build
from tasks.msi import build_installer
from tasks.omnibus import build as omnibus_build


@task
def agent_package(
    ctx,
    flavor=AgentFlavor.base.name,
    release_version="nightly",
    skip_deps=False,
):
    # Build agent
    omnibus_build(
        ctx,
        flavor=flavor,
        release_version=release_version,
        skip_deps=skip_deps,
    )

    # Package Agent into MSI
    agent_msi_build(ctx, release_version=release_version)

    # Package MSI into OCI
    ctx.run('powershell -C "./tasks/winbuildscripts/Generate-OCIPackage.ps1 -package datadog-agent"')


@task
def installer_package(
    ctx,
    release_version="nightly",
    skip_deps=False,
):
    # Build installer
    omnibus_build(
        ctx,
        release_version=release_version,
        skip_deps=skip_deps,
        target_project="installer",
    )

    # Package Insaller into MSI
    build_installer(ctx)

    # Package MSI into OCI
    ctx.run('powershell -C "./tasks/winbuildscripts/Generate-OCIPackage.ps1 -package datadog-installer"')
