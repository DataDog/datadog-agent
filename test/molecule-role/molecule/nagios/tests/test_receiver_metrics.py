import json
import os

from testinfra.utils.ansible_runner import AnsibleRunner

import util

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('agent-nagios-mysql')


def test_container_metrics(host):
    url = "http://localhost:7070/api/topic/sts_multi_metrics?limit=1000"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-sts-multi-metrics.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        def get_keys(m_host):
            return set(
                ''.join(message["message"]["MultiMetric"]["values"].keys())
                for message in json_data["messages"]
                if message["message"]["MultiMetric"]["name"] == "convertedMetric" and
                message["message"]["MultiMetric"]["host"] == m_host
            )

        expected = {'nagios.http.size', 'nagios.ping.pl', 'nagios.http.time', 'nagios.current_load.load15',
                    'nagios.swap_usage.swap', 'nagios.host.pl', 'nagios.root_partition', 'nagios.current_users.users',
                    'nagios.current_load.load1', 'nagios.host.rta', 'nagios.ping.rta', 'nagios.current_load.load5',
                    'nagios.total_processes.procs'}
        assert get_keys("localhost") == expected

    util.wait_until(wait_for_metrics, 180, 3)
