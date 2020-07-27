import json
import os
import re

from testinfra.utils.ansible_runner import AnsibleRunner

import util

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('agent-nagios-mysql')


def _get_key_value(tag_list):
    for key, value in (pair.split(':', 1) for pair in tag_list):
        yield key, value


def _component_data(json_data, type_name, external_id_assert_fn, tags_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyComponent" in p and \
                p["TopologyComponent"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyComponent"]["externalId"]):
            data = json.loads(p["TopologyComponent"]["data"])
            if tags_assert_fn(dict(_get_key_value(data["tags"]))):
                return data
    return None


def test_nagios_mysql(host):
    def assert_topology():
        topo_url = "http://localhost:7070/api/topic/sts_topo_process_agents?limit=1500"
        data = host.check_output('curl "{}"'.format(topo_url))
        json_data = json.loads(data)
        with open("./topic-topo-process-agents.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        external_id_pattern = re.compile(r"urn:container:/agent-nagios-mysql:.*")
        components = [
            {
                "assertion": "Should find the nagios container",
                "type": "container",
                "external_id": lambda e_id: external_id_pattern.findall(e_id),
                "tags": lambda t: t["container_name"] == "ubuntu_nagios_1"
            },
            {
                "assertion": "Should find the mysql container",
                "type": "container",
                "external_id": lambda e_id: external_id_pattern.findall(e_id),
                "tags": lambda t: t["container_name"] == "ubuntu_mysql_1"
            }
        ]

        for c in components:
            print("Running assertion for: " + c["assertion"])
            assert _component_data(
                json_data=json_data,
                type_name=c["type"],
                external_id_assert_fn=c["external_id"],
                tags_assert_fn=c["tags"],
            ) is not None

    util.wait_until(assert_topology, 30, 3)
