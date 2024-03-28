from __future__ import annotations

import json
from typing import TYPE_CHECKING, cast

from tasks.kernel_matrix_testing.vars import COMPONENTS

if TYPE_CHECKING:
    from tasks.kernel_matrix_testing.types import Platforms  # noqa: F401


def get_platforms_file(component: str) -> str:
    if component not in COMPONENTS:
        raise ValueError(f"Unknown component: {component}. Valid ones are {COMPONENTS}")

    return f"test/new-e2e/system-probe/config/platforms-{component}.json"


def get_platforms(component: str) -> Platforms:
    with open(get_platforms_file(component)) as f:
        return cast('Platforms', json.load(f))


def get_merged_platforms() -> Platforms:
    platforms = get_platforms(COMPONENTS[0])
    for component in COMPONENTS[1:]:
        plat = get_platforms(component)

        if plat["url_base"] != platforms["url_base"]:
            raise ValueError(
                f"URL base mismatch for component {component}: {plat['url_base']} != {platforms['url_base']}. All base URLs must be the same"
            )

        platforms['arm64'].update(plat['arm64'])
        platforms['x86_64'].update(plat['x86_64'])

    return platforms
