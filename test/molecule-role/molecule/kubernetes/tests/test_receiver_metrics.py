import json
import os
import util
import pytest

from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('kubernetes-cluster-agent')


def test_agents_running(host):
    url = "http://localhost:7070/api/topic/sts_multi_metrics?limit=1000"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-sts-multi-metrics.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        metrics = {}
        for message in json_data["messages"]:
            for m_name in message["message"]["MultiMetric"]["values"].keys():
                if m_name not in metrics:
                    metrics[m_name] = []

                values = [message["message"]["MultiMetric"]["values"][m_name]]
                metrics[m_name] += values

        for v in metrics["stackstate.agent.running"]:
            assert v == 1.0
        for v in metrics["stackstate.cluster_agent.running"]:
            assert v == 1.0

        # assert that we don't see any datadog metrics
        datadog_metrics = [(key, value) for key, value in metrics.items() if key.startswith("datadog")]
        assert len(datadog_metrics) == 0, 'datadog metrics found in sts_multi_metrics: [%s]' % ', '.join(map(str, datadog_metrics))

    util.wait_until(wait_for_metrics, 60, 3)


def test_agent_http_metrics(host):
    pytest.skip("Disabled HTTP Metrics: https://stackstate.atlassian.net/browse/STAC-13669.")
    url = "http://localhost:7070/api/topic/sts_multi_metrics?limit=1000"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-multi-metrics-http.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        def get_keys():
            return next(set(message["message"]["MultiMetric"]["values"].keys())
                        for message in json_data["messages"]
                        if message["message"]["MultiMetric"]["name"] == "connection metric" and
                        "code" in message["message"]["MultiMetric"]["tags"] and
                        message["message"]["MultiMetric"]["tags"]["code"] == "any"
                        )

        expected = {"http_requests_per_second", "http_response_time_seconds"}

        assert get_keys().pop() in expected

    util.wait_until(wait_for_metrics, 30, 3)


def test_agent_kubernetes_metrics(host):
    url = "http://localhost:7070/api/topic/sts_multi_metrics?limit=1000"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-multi-metrics-kubernetes.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        def contains_key():
            for message in json_data["messages"]:
                if (message["message"]["MultiMetric"]["name"] == "convertedMetric" and
                    "cluster_name" in message["message"]["MultiMetric"]["tags"] and
                    ("kubernetes_state.container.running" in message["message"]["MultiMetric"]["values"].keys() or
                     "kubernetes_state.pod.scheduled" in message["message"]["MultiMetric"]["values"].keys())):
                    return True
            return False

        assert contains_key(), 'No kubernetes metrics found'

    util.wait_until(wait_for_metrics, 60, 3)


def test_agent_kubernetes_state_metrics(host):
    url = "http://localhost:7070/api/topic/sts_multi_metrics?limit=1000"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-multi-metrics-kubernetes_state.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        def contains_key():
            for message in json_data["messages"]:
                if (message["message"]["MultiMetric"]["name"] == "convertedMetric" and
                    "cluster_name" in message["message"]["MultiMetric"]["tags"] and
                    ("kubernetes_state.container.running" in message["message"]["MultiMetric"]["values"] or
                     "kubernetes_state.pod.scheduled" in message["message"]["MultiMetric"]["values"])):
                    return True
            return False

        assert contains_key(), 'No kubernetes_state metrics found'

    util.wait_until(wait_for_metrics, 60, 3)


def test_agent_kubelet_metrics(host):
    url = "http://localhost:7070/api/topic/sts_multi_metrics?limit=1000"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-multi-metrics-kubelet.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        def contains_key():
            for message in json_data["messages"]:
                if (message["message"]["MultiMetric"]["name"] == "convertedMetric" and
                    "namespace" in message["message"]["MultiMetric"]["tags"] and
                    ("kubernetes.kubelet.volume.stats.available_bytes" in message["message"]["MultiMetric"]["values"] or
                     "kubernetes.kubelet.volume.stats.used_bytes" in message["message"]["MultiMetric"]["values"])):
                    return True
            return False

        assert contains_key(), 'No kubelet metrics found'

    util.wait_until(wait_for_metrics, 60, 3)
