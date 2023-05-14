def create_count(metric_name, timestamp, value, tags):
    from datadog_api_client.v2.model.metric_intake_type import MetricIntakeType
    from datadog_api_client.v2.model.metric_point import MetricPoint
    from datadog_api_client.v2.model.metric_series import MetricSeries

    return MetricSeries(
        metric=metric_name,
        type=MetricIntakeType(1),  # count
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
