import json
from typing import Dict, cast
from typing_extensions import TypedDict


class Platforms(TypedDict):
    url_base: str
    x86_64: Dict[str, str]  # noqa: F841
    arm64: Dict[str, str]  # noqa: F841


platforms_file = "test/new-e2e/system-probe/config/platforms.json"


def get_platforms():
    with open(platforms_file) as f:
        return cast(Platforms, json.load(f))
