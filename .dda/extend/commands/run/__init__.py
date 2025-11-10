from __future__ import annotations

from dda.cli.base import dynamic_group


@dynamic_group(
    short_help="Execute commands",
)
def cmd() -> None:
    pass
