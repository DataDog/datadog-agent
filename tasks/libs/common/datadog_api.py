import sys
from datetime import datetime, timedelta, timezone

from invoke.exceptions import Exit


def create_metric(metric_type, metric_name, timestamp, value, tags, unit=None, metric_origin=None):
    """
    - metric_type: See types in the following documentation https://datadoghq.dev/datadog-api-client-python/datadog_api_client.v2.model.html#module-datadog_api_client.v2.model.metric_intake_type
    """
    from datadog_api_client.model_utils import unset
    from datadog_api_client.v2.model.metric_metadata import MetricMetadata
    from datadog_api_client.v2.model.metric_origin import MetricOrigin
    from datadog_api_client.v2.model.metric_point import MetricPoint
    from datadog_api_client.v2.model.metric_series import MetricSeries

    unit = unit or unset
    metadata = unset

    origin_metadata = metric_origin or {
        "origin_product": 10,  # Agent
        "origin_sub_product": 54,  # Agent CI
        "origin_product_detail": 64,  # Gitlab
    }
    metadata = MetricMetadata(origin=MetricOrigin(**origin_metadata))

    return MetricSeries(
        metric=metric_name,
        type=metric_type,
        points=[
            MetricPoint(
                timestamp=timestamp,
                value=value,
            )
        ],
        tags=tags,
        unit=unit,
        metadata=metadata,
    )


def create_count(metric_name, timestamp, value, tags, unit=None):
    from datadog_api_client.v2.model.metric_intake_type import MetricIntakeType

    return create_metric(MetricIntakeType.COUNT, metric_name, timestamp, value, tags, unit)


def create_gauge(metric_name, timestamp, value, tags, unit=None, metric_origin=None):
    from datadog_api_client.v2.model.metric_intake_type import MetricIntakeType

    return create_metric(MetricIntakeType.GAUGE, metric_name, timestamp, value, tags, unit, metric_origin)


def send_metrics(series):
    from datadog_api_client import ApiClient, Configuration
    from datadog_api_client.v2.api.metrics_api import MetricsApi
    from datadog_api_client.v2.model.metric_payload import MetricPayload

    with ApiClient(Configuration(enable_retry=True)) as api_client:
        api_instance = MetricsApi(api_client)
        response = api_instance.submit_metrics(body=MetricPayload(series=series))

        if response["errors"]:
            print(
                f"Error(s) while sending pipeline metrics to the Datadog backend: {response['errors']}", file=sys.stderr
            )
            raise Exit(code=1)

        return response


def send_event(title: str, text: str, tags: list[str] = None):
    """
    Post an event returns "OK" response
    """

    from datadog_api_client import ApiClient, Configuration
    from datadog_api_client.v1.api.events_api import EventsApi
    from datadog_api_client.v1.model.event_create_request import EventCreateRequest

    body = EventCreateRequest(
        title=title,
        text=text,
        tags=tags or [],
    )

    with ApiClient(Configuration(enable_retry=True)) as api_client:
        api_instance = EventsApi(api_client)
        try:
            response = api_instance.create_event(body=body)
        except Exception as e:
            print(f"Error while sending pipeline event to the Datadog backend: {e}", file=sys.stderr)
            raise Exit(code=1) from e

        if response.get("errors", None):
            print(
                f"Error(s) while sending pipeline event to the Datadog backend: {response['errors']}", file=sys.stderr
            )
            raise Exit(code=1)

        return response


def get_ci_pipeline_events(query, days):
    """
    Fetch pipeline events using Datadog CI Visibility API
    """
    from datadog_api_client import ApiClient, Configuration
    from datadog_api_client.v2.api.ci_visibility_pipelines_api import CIVisibilityPipelinesApi

    with ApiClient(Configuration(enable_retry=True)) as api_client:
        api_instance = CIVisibilityPipelinesApi(api_client)
        response = api_instance.list_ci_app_pipeline_events(
            filter_query=query,
            filter_from=(datetime.now() - timedelta(days=days)),
            filter_to=datetime.now(),
            page_limit=5,
        )
        return response


def get_ci_test_events(query, days):
    """
    Fetch test events using Datadog CI Visibility API
    Returns all results by handling pagination automatically
    """
    from datadog_api_client import ApiClient, Configuration
    from datadog_api_client.v2.api.ci_visibility_tests_api import CIVisibilityTestsApi

    all_events = []
    page_cursor = None

    with ApiClient(Configuration(enable_retry=True)) as api_client:
        api = CIVisibilityTestsApi(api_client)

        while True:
            # We filter jobs of a single pipeline by its id and job name
            kwargs = {
                "filter_query": query,
                "page_limit": 1000,
                "filter_from": (datetime.now() - timedelta(days=days)),
                "filter_to": datetime.now(),
            }

            if page_cursor:
                kwargs["page_cursor"] = page_cursor

            response = api.list_ci_app_test_events(**kwargs)
            # Add events from this page to our collection
            if hasattr(response, 'data') and response.data:
                all_events.extend(response.data)

            # Check if there are more pages
            if hasattr(response, 'meta') and hasattr(response.meta, 'page') and hasattr(response.meta.page, 'after'):
                page_cursor = response.meta.page.after
                if not page_cursor:  # No more pages
                    break
            else:
                break  # No pagination metadata, assume single page

    return all_events


def query_metrics(query, from_time, to_time):
    """
    Query Datadog metrics timeseries.

    Args:
        query: Metrics query string (e.g., "avg:metric.name{tag:value} by {group}")
        from_time: Start time as Unix timestamp (seconds) or relative string like "now-1d"
        to_time: End time as Unix timestamp (seconds) or relative string like "now"

    Returns:
        List of series data with scope, values, etc.
    """
    from datadog_api_client import ApiClient, Configuration
    from datadog_api_client.v1.api.metrics_api import MetricsApi

    # Parse relative time strings to Unix timestamps
    def parse_time(time_str):
        if isinstance(time_str, int):
            return time_str
        if time_str == "now":
            return int(datetime.now(timezone.utc).timestamp())
        if time_str.startswith("now-"):
            duration = time_str[4:]
            if duration.endswith("d"):
                delta = timedelta(days=int(duration[:-1]))
            elif duration.endswith("h"):
                delta = timedelta(hours=int(duration[:-1]))
            elif duration.endswith("m"):
                delta = timedelta(minutes=int(duration[:-1]))
            else:
                raise ValueError(f"Unknown time format: {time_str}")
            return int((datetime.now(timezone.utc) - delta).timestamp())
        raise ValueError(f"Unknown time format: {time_str}")

    start = parse_time(from_time)
    end = parse_time(to_time)

    with ApiClient(Configuration(enable_retry=True)) as api_client:
        api_instance = MetricsApi(api_client)
        response = api_instance.query_metrics(
            _from=start,
            to=end,
            query=query,
        )

        # Extract series data from response
        series_list = []
        if not response.series:
            return series_list

        for series in response.series:
            series_data = {
                "scope": series.scope or "",
                "pointlist": series.pointlist or [],
                "expression": series.expression or "",
            }
            series_list.append(series_data)

        return series_list


def query_gate_metrics_for_commit(commit_sha: str, lookback: str = "now-7d") -> dict:
    """
    Query Datadog for static quality gate metrics for a specific commit.

    Uses query_metrics to fetch on_disk_size and on_wire_size metrics
    for an ancestor commit. This provides a consistent source of truth
    for calculating relative size changes.

    Args:
        commit_sha: The git commit SHA to query metrics for
        lookback: How far back to look (default 7 days)

    Returns:
        Dict mapping gate_name -> {'current_on_disk_size': ..., 'current_on_wire_size': ...}
        Empty dict if no metrics found.
    """
    results = {}

    metrics_to_query = {
        'current_on_disk_size': 'datadog.agent.static_quality_gate.on_disk_size',
        'current_on_wire_size': 'datadog.agent.static_quality_gate.on_wire_size',
    }

    for metric_key, metric_name in metrics_to_query.items():
        # Query all gates at once using "by {gate_name}" aggregation
        query = f"avg:{metric_name}{{ci_commit_sha:{commit_sha}}} by {{gate_name}}"

        try:
            series_list = query_metrics(query, lookback, "now")

            for series in series_list:
                # Extract gate_name from scope (e.g., "gate_name:static_quality_gate_agent_deb_amd64")
                scope = series["scope"]
                gate_name = None
                for tag in scope.split(","):
                    if tag.startswith("gate_name:"):
                        gate_name = tag.split(":", 1)[1]
                        break

                if not gate_name:
                    continue

                # Get the most recent non-null value from pointlist
                # Point objects have .value property that returns [timestamp, metric_value]
                pointlist = series["pointlist"]
                for point in reversed(pointlist):
                    if point and point.value and point.value[1] is not None:
                        if gate_name not in results:
                            results[gate_name] = {}
                        results[gate_name][metric_key] = int(point.value[1])
                        break

        except Exception as e:
            print(f"[WARN] Failed to query {metric_name} for commit {commit_sha}: {e}", file=sys.stderr)

    return results
