import time

from datadog_api_client.v1 import ApiClient, Configuration
from datadog_api_client.v1.api.metrics_api import MetricsApi
from retry.api import retry_call


class App:
    def __init__(self):
        self.v1_api_client = ApiClient(Configuration())

    def query_metric(self, name, **kw):
        api_instance = MetricsApi(self.v1_api_client)

        tags = []
        for key, value in kw.items():
            tags.append(f"{key}:{value}")
        if len(tags) == 0:
            tags.append("*")

        response = api_instance.query_metrics(int(time.time()) - 30, int(time.time()), f"{name}{{{','.join(tags)}}}")
        return response

    def wait_for_metric(self, name, tries=30, delay=10, **kw):
        def expect_metric():
            metric = self.query_metric(name, **kw)
            if len(metric.get("series")) == 0:
                raise LookupError(f"no value found in {metric}")
            return metric

        return retry_call(expect_metric, tries=tries, delay=delay)
