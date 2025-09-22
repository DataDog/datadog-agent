import os
import shutil

from invoke.tasks import task

from tasks.flavor import AgentFlavor
from tasks.libs.common.utils import get_version
from tasks.msi import build as build_agent_msi
from tasks.omnibus import build as omnibus_build

# Output directory for package files
OUTPUT_PATH = os.path.join(os.getcwd(), "omnibus", "pkg")
# Omnibus stores files here, e.g. C:\opt\datadog-agent, C:\opt\dataog-installer
OPT_SOURCE_DIR = os.path.join('C:\\', 'opt')


@task
def agent_package(
    ctx,
    flavor=AgentFlavor.base.name,
    skip_deps=False,
    build_upgrade=False,
):
    # Build installer
    # TODO: merge into agent omnibus build
    # TODO: must build installer first so the final build-summary.json
    #       is from the Agent omnibus build
    omnibus_build(
        ctx,
        skip_deps=skip_deps,
        target_project="installer",
    )

    # Build agent
    omnibus_build(
        ctx,
        flavor=flavor,
        skip_deps=skip_deps,
    )

    # Package Agent into MSI
    build_agent_msi(ctx, build_upgrade=build_upgrade)

    # Package MSI into OCI
    if AgentFlavor[flavor] == AgentFlavor.base:
        ctx.run('powershell -C "./tasks/winbuildscripts/Generate-OCIPackage.ps1 -package datadog-agent"')

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
