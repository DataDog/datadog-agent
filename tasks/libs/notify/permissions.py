from collections import defaultdict


def list_permissions(gh, name, all_teams, contributors):
    """
    List contributors and teams to watch out.
    """
    non_contributing_teams = []
    contributors_to_remove = []
    membership = defaultdict(list)
    active_users = gh.get_active_users(duration_days=90)
    print(f"Checking permissions for {name}, {len(active_users)} active users")
    for team in all_teams:
        members = gh.get_direct_team_members(team.slug)
        has_contributors = False
        for member in members:
            if member not in active_users:
                membership[member].append(f"<{team.html_url}|{team.slug}>")
                if team.slug == contributors:
                    contributors_to_remove.append(member)
            else:
                has_contributors = True
        if not has_contributors:
            non_contributing_teams.append(f"<{team.html_url}|{team.slug}>")
    return non_contributing_teams, contributors_to_remove, membership
