import os
import shutil

from invoke.tasks import task

from tasks.flavor import AgentFlavor
from tasks.libs.common.utils import get_version
from tasks.msi import build as build_agent_msi
from tasks.omnibus import build as omnibus_build

# Output directory for package files
OUTPUT_PATH = os.path.join(os.getcwd(), "omnibus", "pkg")
# Omnibus stores files here, e.g. C:\opt\datadog-agent, C:\opt\datadog-installer
OPT_SOURCE_DIR = os.path.join("C:\\", "opt")


@task
def agent_package(
    ctx,
    flavor=AgentFlavor.base.name,
    skip_deps=False,
    build_upgrade=False,
):
    # Build agent
    omnibus_build(
        ctx,
        flavor=flavor,
        skip_deps=skip_deps,
    )

    # Move the installer binary to a separate folder
    os.makedirs(os.path.join(OPT_SOURCE_DIR, "datadog-installer"))
    shutil.move(
        os.path.join(OPT_SOURCE_DIR, "datadog-agent", "datadog-installer.exe"),
        os.path.join(OPT_SOURCE_DIR, "datadog-installer"),
    )

    # Package Agent into MSI
    build_agent_msi(ctx, build_upgrade=build_upgrade)

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
