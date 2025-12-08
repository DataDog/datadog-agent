from __future__ import annotations

from dda.cli.base import dynamic_group


@dynamic_group(
    short_help="Lab environments to experiment with the Agent",
)
def cmd() -> None:
    pass
