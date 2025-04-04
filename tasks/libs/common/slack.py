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
            "emoji": True,
        },
    }


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
