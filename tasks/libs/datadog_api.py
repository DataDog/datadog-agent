def create_count(metric_name, timestamp, value, tags):
    from datadog_api_client.v2.model.metric_intake_type import MetricIntakeType
    from datadog_api_client.v2.model.metric_point import MetricPoint
    from datadog_api_client.v2.model.metric_series import MetricSeries

    return MetricSeries(
        metric=metric_name,
        # 1 is the count type
        # https://datadoghq.dev/datadog-api-client-python/datadog_api_client.v2.model.html#module-datadog_api_client.v2.model.metric_intake_type
        type=MetricIntakeType(1),
        points=[
            MetricPoint(
                timestamp=timestamp,
                value=value,
            )
        ],
        tags=tags,
    )


def create_gauge(metric_name, timestamp, value, tags):
    from datadog_api_client.v2.model.metric_intake_type import MetricIntakeType
    from datadog_api_client.v2.model.metric_point import MetricPoint
    from datadog_api_client.v2.model.metric_series import MetricSeries

    return MetricSeries(
        metric=metric_name,
        # 3 is the gauge type
        # https://datadoghq.dev/datadog-api-client-python/datadog_api_client.v2.model.html#module-datadog_api_client.v2.model.metric_intake_type
        type=MetricIntakeType(3),
        points=[
            MetricPoint(
                timestamp=timestamp,
                value=value,
            )
        ],
        tags=tags,
    )


def send_metrics(series):
    from datadog_api_client import ApiClient, Configuration
    from datadog_api_client.v2.api.metrics_api import MetricsApi
    from datadog_api_client.v2.model.metric_payload import MetricPayload

    configuration = Configuration()
    with ApiClient(configuration) as api_client:
        api_instance = MetricsApi(api_client)
        return api_instance.submit_metrics(body=MetricPayload(series=series))
