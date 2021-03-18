import os
import json

import testinfra.utils.ansible_runner

import util

testinfra_hosts = testinfra.utils.ansible_runner.AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('all')


def test_hosts_file(host):
    f = host.file('/etc/hosts')

    assert f.exists
    assert f.user == 'root'
    assert f.group == 'root'


def test_docker_swarm_metrics(host):
    hostname = host.ansible.get_variables()["inventory_hostname"]
    url = "http://localhost:7070/api/topic/sts_multi_metrics?limit=1000"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-docker-swarm-sts-multi-metrics.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        def get_keys():
            # Check for a swarm service which all metrics are we returning
            # as an example we are taking for nginx
            return set(
                ''.join(message["message"]["MultiMetric"]["values"].keys())
                for message in json_data["messages"]
                if message["message"]["MultiMetric"]["name"] == "convertedMetric" and
                message["message"]["MultiMetric"]["tags"]["serviceName"] == "nginx"
            )

        expected = {'swarm.service.desired_replicas', 'swarm.service.running_replicas'}
        assert all([expectedMetric for expectedMetric in expected if expectedMetric in get_keys()])

    util.wait_until(wait_for_metrics, 180, 3)
