from __future__ import annotations

from typing import TYPE_CHECKING

from dda.cli.base import dynamic_command, pass_app
from utils.docs.deps import COMMAND_DEPENDENCIES

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(short_help="Check documentation links", dependencies=COMMAND_DEPENDENCIES)
@pass_app
def cmd(app: Application) -> None:
    """
    Check the links of the built documentation.
    """
    from utils.docs.links import check_links

    check_links(app)
