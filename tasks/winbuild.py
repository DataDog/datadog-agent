import os
import shutil

from invoke.tasks import task

from tasks.flavor import AgentFlavor
from tasks.libs.common.utils import get_version
from tasks.msi import build as build_agent_msi
from tasks.msi import build_installer as build_installer_msi
from tasks.omnibus import build as omnibus_build

# Output directory for package files
OUTPUT_PATH = os.path.join(os.getcwd(), "omnibus", "pkg")
# Omnibus stores files here, e.g. C:\opt\datadog-agent, C:\opt\dataog-installer
OPT_SOURCE_DIR = os.path.join('C:\\', 'opt')


@task
def agent_package(
    ctx,
    flavor=AgentFlavor.base.name,
    release_version="nightly",
    skip_deps=False,
    build_upgrade=False,
):
    # Build agent
    omnibus_build(
        ctx,
        flavor=flavor,
        release_version=release_version,
        skip_deps=skip_deps,
    )

    # Build installer
    omnibus_build(
        ctx,
        release_version=release_version,
        skip_deps=skip_deps,
        target_project="installer",
    )

    # Package Agent into MSI
    build_agent_msi(ctx, release_version=release_version, build_upgrade=build_upgrade)

    # Package MSI into OCI
    if AgentFlavor[flavor] == AgentFlavor.base:
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
    build_installer_msi(ctx)

    # Package MSI into OCI
    ctx.run('powershell -C "./tasks/winbuildscripts/Generate-OCIPackage.ps1 -package datadog-installer"')

    # Copy installer.exe to the output dir so it can be deployed as the bootstrapper
    agent_version = get_version(
        ctx,
        include_git=True,
        url_safe=True,
        include_pipeline_id=True,
    )
    shutil.copy2(
        os.path.join(OPT_SOURCE_DIR, "datadog-installer\\datadog-installer.exe"),
        os.path.join(OUTPUT_PATH, f"datadog-installer-{agent_version}-1-x86_64.exe"),
    )
