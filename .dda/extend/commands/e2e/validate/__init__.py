from __future__ import annotations

from dda.cli.base import dynamic_group


@dynamic_group(short_help="Static analysis commands")
def cmd() -> None:
    """
    Commands related to static analysis.
    """
    pass
