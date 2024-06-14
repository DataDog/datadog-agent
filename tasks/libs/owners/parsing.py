from __future__ import annotations

from collections import Counter
from typing import Any


def read_owners(owners_file: str) -> Any:
    from codeowners import CodeOwners

    with open(owners_file) as f:
        return CodeOwners(f.read())


def search_owners(search: str, owners_file: str) -> list[str]:
    parsed_owners = read_owners(owners_file)
    # owners.of returns a list in the form: [('TEAM', '@DataDog/agent-build-and-releases')]
    return [owner[1] for owner in parsed_owners.of(search)]


def list_owners(owners_file=".github/CODEOWNERS"):
    owners = read_owners(owners_file)
    for path in owners.paths:
        for team in path[2]:
            yield team[1].casefold().replace('@datadog/', '')


def most_frequent_agent_team(teams):
    agent_teams = list(list_owners())
    c = Counter(teams)
    for team in c.most_common():
        if team[0] in agent_teams:
            return team[0]
    return 'triage'
