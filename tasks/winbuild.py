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
def test_boto(ctx):
    import sys
    import botocore.credentials as creds

    # Get the AWS credentials that we need to supply to jsign in order to sign a binary with the key.

    # This supports the case where the module is running on an instance/context which is already the desired role
    # and can access the AWS special IP addresses. That is our most important case. Using boto3 + STS + IAM to
    # create tokens is preferred, but is complicated for our case as AssumeRole into the current context's role is
    # not allowed.
    print('CELIAN start creds', file=sys.stderr)
    role_fetcher = creds.InstanceMetadataFetcher(timeout=10, num_attempts=2)
    print('CELIAN role fetcher', file=sys.stderr)
    provider = creds.InstanceMetadataProvider(iam_role_fetcher=role_fetcher)
    print('CELIAN provider', file=sys.stderr)
    credential_data = provider.load().get_frozen_credentials()
    print('CELIAN credential_data', file=sys.stderr)

    # Extract the necessary data for jsign
    # access_key_id = credential_data.access_key
    # secret_access_key = credential_data.secret_key
    aws_token = credential_data.token

    print(f'CELIAN ok: {len(aws_token)}', file=sys.stderr)


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

    # Build installer
    omnibus_build(
        ctx,
        skip_deps=skip_deps,
        target_project="installer",
    )

    # Package Agent into MSI
    build_agent_msi(ctx, build_upgrade=build_upgrade)

    # Package MSI into OCI
    if AgentFlavor[flavor] == AgentFlavor.base:
        ctx.run('powershell -C "./tasks/winbuildscripts/Generate-OCIPackage.ps1 -package datadog-agent"')


@task
def installer_package(
    ctx,
    skip_deps=False,
):
    # Build installer
    omnibus_build(
        ctx,
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
