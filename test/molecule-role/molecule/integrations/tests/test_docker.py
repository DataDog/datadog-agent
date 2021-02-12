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

        container_event = {
            "host": hostname,
            "title": "docker.restart",
            "message": "Container nginx-1 restarted",
            "eventType": "service-check",
            "name": "service-check.service-check",
            "tags": {
                "short_image": "nginx",
                "source_type_name": "service-check",
                "image_tag": "1.14.2",
                "docker_image": "nginx:1.14.2",
                "status": "WARNING",
                "container_name": "nginx-1",
            },
        }
        assert util.event_data(container_event, json_data, hostname) is not None

    util.wait_until(wait_for_events, 180, 3)
