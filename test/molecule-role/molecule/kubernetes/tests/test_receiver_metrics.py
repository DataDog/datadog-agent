import json
import os
import util

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
        datadog_metrics = [(key, value) for key, value in metrics.iteritems() if key.startswith("datadog")]
        assert len(datadog_metrics) == 0, 'datadog metrics found in sts_multi_metrics: [%s]' % ', '.join(map(str, datadog_metrics))

    util.wait_until(wait_for_metrics, 60, 3)
