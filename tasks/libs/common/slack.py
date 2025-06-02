from tasks.libs.types.types import PermissionCheck


def header_block(text: str) -> dict:
    """
    Create a Slack block with a header.

    Args:
        text (str): The text to be displayed as a header.

    Returns:
        dict: A dictionary representing the Slack block.
    """
    return {
        "type": "header",
        "text": {
            "type": "plain_text",
            "text": text,
        },
    }


def format_teams(name: str, check: PermissionCheck, all_teams: list) -> list:
    """
    Add teams as blocks for Slack message.
    """
    block = "Teams:\n"
    blocks = []
    MAX_LENGTH = 3000
    current_size = 0
    members_count = 0
    for team in all_teams:
        t = f" - <{team.html_url}|{team.slug}>[{team.members_count}]: {permission_str(name, check, team)}\n"
        members_count += team.members_count
        if current_size + len(t) > MAX_LENGTH:
            blocks.append(markdown_block(block))
            block, current_size = t, 0
        else:
            block += t
            current_size += len(t)
    blocks.append(markdown_block(block))
    blocks.append(markdown_block(f"Total members <= {members_count}\n"))
    return blocks


def markdown_block(text: str) -> dict:
    """
    Create a Slack block with markdown text.

    Args:
        text (str): The text to be formatted in markdown.

    Returns:
        dict: A dictionary representing the Slack block.
    """
    return {
        "type": "section",
        "text": {
            "type": "mrkdwn",
            "text": text,
        },
    }


def permission_str(name, check, team):
    """
    Translate the permission value to a string.
    """
    target = f"datadog/{name}" if check == PermissionCheck.REPO else "datadog/datadog-agent"
    team_permission = team.get_repo_permission(target)
    if team_permission is None:
        return 'none'
    if team_permission.admin:
        return 'admin'
    if team_permission.maintain:
        return 'maintain'
    if team_permission.push:
        return 'write'
    if team_permission.pull:
        return 'read'
    return 'triage'
