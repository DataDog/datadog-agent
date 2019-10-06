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

    util.wait_until(wait_for_metrics, 30, 3)
