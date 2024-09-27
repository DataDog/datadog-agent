import os
import re
import sys

try:
    from atlassian import Jira
except ImportError:
    pass
from invoke import task

from tasks.libs.notify.jira_failing_tests import get_failing_tests_names


def get_jira():
    username = os.environ['ATLASSIAN_USERNAME']
    password = os.environ['ATLASSIAN_PASSWORD']
    jira = Jira(url="https://datadoghq.atlassian.net", username=username, password=password)

    return jira


def close_issue(jira, issue_key: str, verbose_test: str):
    print('Marking as done issue', issue_key, 'for test', verbose_test)

    jira.issue_add_comment(issue_key, 'Marking this issue as done since the test is not failing anymore')
    jira.issue_transition(issue_key, 'Wont Do')


@task
def close_failing_tests_stale_issues(_):
    """
    Will mark as done all issues created by the [failed parent tests workflow](https://app.datadoghq.com/workflow/62670e82-8416-459b-bf74-9367b8a69277) that are stale.
    Stale is an issue:
    - In the "To Do" section of a project
    - Where the test has not failed since 28 days
    - That has no comment other than the bot's comments

    This task is executed periodically.
    """

    re_test_name = re.compile('Test name: (.*)\n')

    still_failing = get_failing_tests_names()
    jira = get_jira()

    print('Getting potential issues to close')
    issues = jira.jql('status = "To Do" AND summary ~ "Failed agent CI test"')['issues']

    print(f'{len(issues)} failing test cards found')

    n_closed = 0
    for issue in issues:
        try:
            # No comment other than the bot's comments
            comments = issue['fields']['comment']['comments']
            has_no_comments = True
            test_name = None
            for comment in comments:
                # This is not a bot message
                if 'robot' not in comment['author']['displayName'].casefold():
                    has_no_comments = False
                    break

                test_name_match = re_test_name.findall(comment['body'])
                if test_name_match:
                    test_name = test_name_match[0]

            if has_no_comments and test_name and test_name not in still_failing:
                close_issue(jira, issue['key'], test_name)
                n_closed += 1
        except Exception as e:
            print(f'Error processing issue {issue["key"]}: {e}', file=sys.stderr)

    print(f'Closed {n_closed} issues without failing tests')
