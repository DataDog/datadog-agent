import json
import os
import util

from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('kubernetes-cluster-agent')


def test_generic_events(host):
    url = "http://localhost:7070/api/topic/sts_generic_events?limit=1000"

    def wait_for_events():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-sts-generic-events.json", 'w') as f:
            json.dump(json_data, f, indent=4)


    util.wait_until(wait_for_events, 60, 3)


def test_topology_events(host):
    url = "http://localhost:7070/api/topic/sts_topology_events?limit=1000"

    def wait_for_topology_events():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-sts-topology-events.json", 'w') as f:
            json.dump(json_data, f, indent=4)

    util.wait_until(wait_for_topology_events, 60, 3)
