import os


def _stringify_config(config_dict):
    """
    Takes a config dict of the following form:
    {
        "xxx_VERSION": Version(major: x, minor: y, patch: z, rc: t, prefix: "pre"),
        "xxx_HASH": "hashvalue",
        ...
    }

    and transforms all VERSIONs into their string representation (using the Version object's __str__).
    """
    return {key: str(value) for key, value in config_dict.items()}


def list_not_closed_qa_cards(version):
    username = os.environ['ATLASSIAN_USERNAME']
    password = os.environ['ATLASSIAN_PASSWORD']
    from atlassian import Jira

    jira = Jira(url="https://datadoghq.atlassian.net", username=username, password=password, cloud=True)
    jql = f'labels in (ddqa) and labels not in (test_ignore) and labels in ({version}-qa) and status not in ((Done, DONE, "Won\'t Fix", "WON\'T FIX", "In Progress", "Testing/Review", "In review", "✅ Done", "won\'t do", Duplicate, Closed, "NOT DOING", not-do, canceled, QA)) order by created desc'
    response = jira.enhanced_jql(jql)
    return response['issues']
