from __future__ import annotations

from dda.cli.base import dynamic_group

# Import providers to register them
from lab.providers.local import kind  # noqa: F401


@dynamic_group(
    short_help="Lab environments to experiment with the Agent",
)
def cmd() -> None:
    pass
