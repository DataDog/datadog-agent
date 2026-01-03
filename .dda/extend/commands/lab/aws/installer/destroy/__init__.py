# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

from typing import TYPE_CHECKING

from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(
    short_help="Destroy an installer lab on AWS",
)
@pass_app
def cmd(
    app: Application,
) -> None:
    """
    Destroy an installer lab environment on AWS.

    This command destroys the Pulumi stack and all associated AWS resources.

    Examples:

        # Destroy the installer lab
        dda lab aws installer destroy
    """
    import sys
    from pathlib import Path

    repo_root = Path(__file__).parent.parent.parent.parent.parent.parent.parent.parent
    e2e_tasks_path = repo_root / "test" / "e2e-framework"
    if str(e2e_tasks_path) not in sys.path:
        sys.path.insert(0, str(e2e_tasks_path))

    from invoke.context import Context

    from tasks.aws.installer import destroy_installer_lab as destroy_installer_task

    ctx = Context()

    app.display_info("Destroying installer lab on AWS...")

    try:
        destroy_installer_task(ctx)
        app.display_success("Installer lab destroyed successfully!")
    except SystemExit as e:
        if e.code != 0:
            app.abort(f"Failed to destroy installer lab (exit code: {e.code})")
    except Exception as e:
        app.abort(f"Failed to destroy installer lab: {e}")

