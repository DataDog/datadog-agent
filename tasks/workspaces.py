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
dda inv install-tools
echo 'export PATH=$PATH:/home/bits/go/bin' >> ~/.zshrc
"""


@task
def create(ctx, name: str):
    """
    Create a workspace and bootstrap dda on it (download dda, install-tools, PATH in ~/.zshrc).

    Requires the `workspaces` CLI on your PATH.
    """
    name_s = name.strip()
    if not name_s:
        raise Exit("workspace name is required")

    quoted = shlex.quote(name_s)
    ctx.run(f"workspaces create {quoted}")

    ctx.run(
        f"ssh {quoted} bash -s <<'EOF'\n{_REMOTE_SETUP}\nEOF",
    )

    print(color_message(f"SSH: ssh {name_s}", Color.GREEN))


@task
def delete(ctx, name: str):
    """Delete a workspace via `workspaces delete`."""
    name = name.strip()
    if not name:
        raise Exit("workspace name is required")

    ctx.run(f"workspaces delete {shlex.quote(name)}", pty=True)
