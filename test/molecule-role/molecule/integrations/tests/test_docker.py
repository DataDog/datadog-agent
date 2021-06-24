import json
import os

from testinfra.utils.ansible_runner import AnsibleRunner

import util

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('agent-integrations-mysql')


def test_container_restart_events(host):
    hostname = host.ansible.get_variables()["inventory_hostname"]
    url = "http://localhost:7070/api/topic/sts_generic_events?limit=1000"

    def wait_for_events():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)

        for message in json_data["messages"]:
            p = message["message"]
            if "GenericEvent" in p:
                event = p["GenericEvent"]
                if event["host"] == hostname and event["title"] == "docker.restart":
                    assert event["eventType"] == "service-check"
                    assert event["message"] == "Container nginx-1 restarted"
                    assert event["tags"]["container_name"] == "nginx-1"
                    assert event["tags"]["status"] == "WARNING"
                    assert event["tags"]["docker_image"] == "nginx:1.14.2"

    util.wait_until(wait_for_events, 180, 3)
