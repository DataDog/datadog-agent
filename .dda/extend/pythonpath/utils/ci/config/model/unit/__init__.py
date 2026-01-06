from __future__ import annotations

from typing import Annotated

from msgspec import Meta, Struct, field

from utils.ci.config.model.unit.common import default_settings
from utils.ci.config.model.unit.gitlab import GitLabUnitProviderConfig


class CIUnitTrigger(Struct, **default_settings):
    """
    This defines the base conditions under which a unit will run.
    """

    patterns: Annotated[
        list[Annotated[str, Meta(min_length=1)]],
        Meta(description="Glob patterns relative to the repository root that will cause the unit to run"),
    ]
    watch_config: Annotated[
        bool,
        Meta(description="Whether to add the unit's config file to the trigger's patterns"),
    ] = field(name="watch-config", default=True)
    allow_manual: Annotated[
        bool,
        Meta(description="Whether the unit can be triggered manually"),
    ] = field(name="allow-manual", default=True)
    allow_tags: Annotated[
        bool,
        Meta(description="Whether the unit can be triggered by tags"),
    ] = field(name="allow-tags", default=False)

    def __post_init__(self) -> None:
        patterns_to_indices = {}
        for i, pattern in enumerate(self.patterns):
            patterns_to_indices.setdefault(pattern, []).append(i)

        errors = []
        for pattern, indices in patterns_to_indices.items():
            if len(indices) > 1:
                msg = f"Duplicate pattern `{pattern}` at indices {', '.join(str(i) for i in indices)}"
                errors.append(msg)

        if errors:
            raise ValueError("\n".join(errors))


class CIUnit(Struct, **default_settings):
    """
    A CI unit is a collection of jobs that run independently of other units based on user-defined conditions.
    """

    name: Annotated[
        str,
        Meta(
            min_length=1,
            description=(
                "A human-readable name for the unit that is available at runtime as the "
                "`UNIT_DISPLAY_NAME` environment variable"
            ),
        ),
    ]
    description: Annotated[
        str,
        Meta(
            min_length=1,
            description="A short description of the unit that is used for documentation purposes",
        ),
    ]
    trigger: Annotated[
        CIUnitTrigger,
        Meta(description="The configuration that defines when the unit will run"),
    ]
    provider: Annotated[
        GitLabUnitProviderConfig,
        Meta(description="The configuration for the unit's provider"),
    ]
