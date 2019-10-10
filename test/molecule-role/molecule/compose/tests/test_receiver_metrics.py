import json
import os
import util
from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('trace-java-demo')


def test_container_metrics(host):
    url = "http://localhost:7070/api/topic/sts_multi_metrics?limit=1000"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-multi-metrics.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        def get_keys(m_host):
            return next(set(message["message"]["MultiMetric"]["values"].keys())
                        for message in json_data["messages"]
                        if message["message"]["MultiMetric"]["name"] == "containerMetrics" and
                        message["message"]["MultiMetric"]["host"] == m_host
                        )

        expected = {"netRcvdPs", "memCache", "totalPct", "wbps", "systemPct", "rbps", "memRss", "netSentBps",
                    "netSentPs", "netRcvdBps", "userPct"}
        assert get_keys("trace-java-demo") == expected

    util.wait_until(wait_for_metrics, 60, 3)


def test_agents_running(host):
    url = "http://localhost:7070/api/topic/sts_metrics?limit=1000"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-sts_metrics.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        metrics = {}
        for message in json_data["messages"]:
            metric = message["message"]["Metric"]

            m_name = metric["name"]
            # m_host = metric["host"]

            if m_name not in metrics:
                metrics[m_name] = []

            values = [value["value"] for value in metric["value"]]
            metrics[m_name] += values

        # Assert that we don't see any Datadog metrics
        datadog_metrics = [(key, value) for key, value in metrics.iteritems() if key.startswith("datadog")]
        assert len(datadog_metrics) == 0, 'Datadog metrics found in sts_metrics: [%s]' % ', '.join(map(str, datadog_metrics))

    util.wait_until(wait_for_metrics, 60, 3)
