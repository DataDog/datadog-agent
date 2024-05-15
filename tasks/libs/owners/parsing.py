from __future__ import annotations

from typing import Any


def read_owners(owners_file: str) -> Any:
    from codeowners import CodeOwners

    with open(owners_file) as f:
        return CodeOwners(f.read())


def search_owners(search: str, owners_file: str) -> list[str]:
    parsed_owners = read_owners(owners_file)
    # owners.of returns a list in the form: [('TEAM', '@DataDog/agent-build-and-releases')]
    return [owner[1] for owner in parsed_owners.of(search)]
