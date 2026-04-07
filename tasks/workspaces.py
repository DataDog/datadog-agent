"""
Tasks for managing remote developer workspaces via the `workspaces` CLI.
"""

import shlex

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message

_DDA_TARBALL_URL = (
    "https://github.com/DataDog/datadog-agent-dev/releases/latest/download/dda-x86_64-unknown-linux-gnu.tar.gz"
)

_REMOTE_SETUP = f"""set -euo pipefail
cd ~
wget {_DDA_TARBALL_URL}
tar -xvzf dda-x86_64-unknown-linux-gnu.tar.gz
sudo mv ./dda /usr/local/bin
( cd ~/dd/datadog-agent && dda inv install-tools )
echo 'export PATH=$PATH:/home/bits/go/bin' >> ~/.zshrc
"""


@task
def create(ctx, name: str, branch: str = ""):
    """
    Create a workspace and bootstrap dda on it (download dda, install-tools, PATH in ~/.zshrc).

    Pass ``branch`` to add ``--branch <branch>`` to ``workspaces create`` (omit for default).

    Requires the `workspaces` CLI on your PATH.
    """
    name_s = name.strip()
    if not name_s:
        raise Exit("workspace name is required")

    quoted = shlex.quote(name_s)
    ssh_host = f'workspace-{quoted}'
    create_cmd = f"workspaces create {quoted}"
    branch_s = branch.strip()
    if branch_s:
        create_cmd += f" --branch {shlex.quote(branch_s)}"
    ctx.run(create_cmd)

    ctx.run(
        f"ssh {ssh_host} bash -s <<'EOF'\n{_REMOTE_SETUP}\nEOF",
    )

    print(f'You can authenticate with AWS using: dda inv workspaces.aws-auth {quoted}')
    print(color_message(f"SSH: ssh {ssh_host}", Color.GREEN))


@task
def delete(ctx, name: str):
    """Delete a workspace via `workspaces delete`."""
    name_s = name.strip()
    if not name_s:
        raise Exit("workspace name is required")

    ctx.run(f"workspaces delete {shlex.quote(name_s)}", pty=True)


@task
def cmd(ctx, name: str, cmd: str):
    """
    Will execute command
    """
    ctx.run(f"ssh workspace-{name} bash -c {shlex.quote(cmd)}")


@task
def aws_auth(ctx, name: str, account: str = 'sso-agent-sandbox-account-admin-8h'):
    """
    Will authenticate with AWS on the workspace
    """
    ctx.run(f"ssh workspace-{name} bash -c 'aws-vault exec {account} -- echo Done'")
