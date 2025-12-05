from __future__ import annotations

from msgspec import Struct

# TODO: use a metaclass when we upgrade msgspec to 0.20.0
default_settings = {
    "forbid_unknown_fields": True,
    "frozen": True,
    "kw_only": True,
    "omit_defaults": True,
}


class CIUnitProviderConfig(Struct, tag_field="name", **default_settings):
    """
    This defines the base configuration for a unit's provider.
    """
