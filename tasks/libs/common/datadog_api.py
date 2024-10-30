import sys

from invoke.exceptions import Exit


def create_metric(metric_type, metric_name, timestamp, value, tags, unit=None):
    """
    - metric_type: See types in the following documentation https://datadoghq.dev/datadog-api-client-python/datadog_api_client.v2.model.html#module-datadog_api_client.v2.model.metric_intake_type
    """
    from datadog_api_client.model_utils import unset
    from datadog_api_client.v2.model.metric_point import MetricPoint
    from datadog_api_client.v2.model.metric_series import MetricSeries

    unit = unit or unset

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
    )


def create_count(metric_name, timestamp, value, tags, unit=None):
    from datadog_api_client.v2.model.metric_intake_type import MetricIntakeType

    return create_metric(MetricIntakeType.COUNT, metric_name, timestamp, value, tags, unit)


def create_gauge(metric_name, timestamp, value, tags, unit=None):
    from datadog_api_client.v2.model.metric_intake_type import MetricIntakeType

    return create_metric(MetricIntakeType.GAUGE, metric_name, timestamp, value, tags, unit)


def send_metrics(series):
    from datadog_api_client import ApiClient, Configuration
    from datadog_api_client.v2.api.metrics_api import MetricsApi
    from datadog_api_client.v2.model.metric_payload import MetricPayload

    configuration = Configuration()
    with ApiClient(configuration) as api_client:
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

    configuration = Configuration()
    with ApiClient(configuration) as api_client:
        api_instance = EventsApi(api_client)
        response = api_instance.create_event(body=body)

        if response["errors"]:
            print(
                f"Error(s) while sending pipeline event to the Datadog backend: {response['errors']}", file=sys.stderr
            )
            raise Exit(code=1)

        return response
