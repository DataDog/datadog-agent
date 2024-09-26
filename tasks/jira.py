import os
import re
import sys

try:
    from atlassian import Jira
except ImportError:
    pass
from datadog_api_client import ApiClient, Configuration
from datadog_api_client.v2.api.ci_visibility_tests_api import CIVisibilityTestsApi
from datadog_api_client.v2.model.ci_app_aggregate_sort import CIAppAggregateSort
from datadog_api_client.v2.model.ci_app_aggregation_function import CIAppAggregationFunction
from datadog_api_client.v2.model.ci_app_compute import CIAppCompute
from datadog_api_client.v2.model.ci_app_compute_type import CIAppComputeType
from datadog_api_client.v2.model.ci_app_query_options import CIAppQueryOptions
from datadog_api_client.v2.model.ci_app_sort_order import CIAppSortOrder
from datadog_api_client.v2.model.ci_app_tests_aggregate_request import CIAppTestsAggregateRequest
from datadog_api_client.v2.model.ci_app_tests_group_by import CIAppTestsGroupBy
from datadog_api_client.v2.model.ci_app_tests_query_filter import CIAppTestsQueryFilter
from invoke import task


def get_failing_tests_names() -> set[str]:
    """
    Returns the names of the failing tests for the last 28 days
    """

    print('Getting failing tests for the last 28 days')
    body = CIAppTestsAggregateRequest(
        compute=[
            CIAppCompute(
                aggregation=CIAppAggregationFunction.COUNT,
                metric="@test.full_name",
                type=CIAppComputeType.TOTAL,
            ),
        ],
        filter=CIAppTestsQueryFilter(
            _from="now-28d",
            query="@test.service:datadog-agent @git.branch:main @test.status:fail",
            to="now",
        ),
        group_by=[
            CIAppTestsGroupBy(
                facet="@test.full_name",
                limit=10000,
                sort=CIAppAggregateSort(
                    order=CIAppSortOrder.DESCENDING,
                ),
                total=False,
            ),
        ],
        options=CIAppQueryOptions(
            timezone="GMT",
        ),
    )

    configuration = Configuration()
    with ApiClient(configuration) as api_client:
        api_instance = CIVisibilityTestsApi(api_client)
        response = api_instance.aggregate_ci_app_test_events(body=body)
        result = response['data']['buckets']
        tests = {row['by']['@test.full_name'].removeprefix('github.com/DataDog/datadog-agent/') for row in result}

        return tests


def get_failed_tests_issues() -> list[dict]:
    print('Getting potential issues to close')

    username = os.environ['ATLASSIAN_USERNAME']
    password = os.environ['ATLASSIAN_PASSWORD']
    jira = Jira(url="https://datadoghq.atlassian.net", username=username, password=password)

    issues = jira.jql('status = "To Do" AND summary ~ "Failed agent CI test"')['issues']

    return issues


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
    from atlassian import Jira

    username = os.environ['ATLASSIAN_USERNAME']
    password = os.environ['ATLASSIAN_PASSWORD']
    jira = Jira(url="https://datadoghq.atlassian.net", username=username, password=password)
    jira.issue_add_comment('CELIANTST-1', 'Test comment')
    exit()
    robot_name = 'Robot Slack SRE'
    re_test_name = re.compile('Test name: (.*)\n')

    still_failing = get_failing_tests_names()
    issues = get_failed_tests_issues()

    print(f'{len(issues)} failing test cards found')

    n_closed = 0
    for issue in issues:
        try:
            # No comment other than the bot's comments
            comments = issue['fields']['comment']['comments']
            has_no_comments = True
            test_name = None
            for comment in comments:
                if comment['author']['displayName'] != robot_name:
                    has_no_comments = False
                    break

                test_name_match = re_test_name.findall(comment['body'])
                if test_name_match:
                    test_name = test_name_match[0]

            if has_no_comments and test_name and test_name not in still_failing:
                print(f'Closing issue {issue["key"]} for test {test_name}')
                # todo
                n_closed += 1
        except Exception as e:
            print(f'Error processing issue {issue["key"]}: {e}', file=sys.stderr)

    print(f'Closed {n_closed} issues without failing tests')
