try:
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
except ImportError:
    pass


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
