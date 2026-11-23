"""Shared result type for per-OS healthcheck backends."""

from typing import NamedTuple


class Failure(NamedTuple):
    """A single unsafe or unmet library dependency.

    Backends return a set of Failures; duplicates (same library/dep/resolved
    triple appearing more than once in ldd/otool output) are silently deduped.
    """

    current_library: str
    dep_name: str
    resolved_path: str


Failures = set[Failure]
