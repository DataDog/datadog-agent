from __future__ import annotations

from dda.cli.base import dynamic_group


@dynamic_group(
    short_help="Validate the repository",
)
def cmd() -> None:
    pass
