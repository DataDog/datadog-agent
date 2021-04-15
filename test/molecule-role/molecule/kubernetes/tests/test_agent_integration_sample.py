import json
import os

import util
import integration_sample

from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('kubernetes-cluster-agent')


def _get_key_value(tag_list):
    for key, value in (pair.split(':', 1) for pair in tag_list):
        yield key, value


def kubernetes_event_data(event, json_data):
    for message in json_data["messages"]:
        p = message["message"]
        if "GenericEvent" in p:
            _data = p["GenericEvent"]
            if _data == dict(_data, **event):
                return _data
    return None


def test_agent_integration_sample_metrics(host):
    expected = {'system.cpu.usage', 'location.availability', '2xx.responses', '5xx.responses', 'check_runs'}
    util.assert_metrics(host, "agent-integration-sample", expected)


def test_agent_integration_sample_topology(host):
    expected_components = integration_sample.get_agent_integration_sample_expected_topology()
    util.assert_topology(host, "agent-integration-sample", "sts_topo_agent_integrations", expected_components)


def test_agent_integration_sample_events(host):
    url = "http://localhost:7070/api/topic/sts_generic_events?limit=1000"

    def wait_for_events():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-agent-integration-sample-sts-generic-events.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        service_event = {
            "name": "service-check.service-check",
            "title": "stackstate.agent.check_status",
            "eventType": "service-check",
            "tags": {
                "source_type_name": "service-check",
                "status": "OK",
                "check": "cpu"
            },
        }
        assert kubernetes_event_data(service_event, json_data) is not None

        http_event = {
            "name": "HTTP_TIMEOUT",
            "title": "URL timeout",
            "eventType": "HTTP_TIMEOUT",
            "tags": {
                "source_type_name": "HTTP_TIMEOUT"
            },
            "message": "Http request to http://localhost timed out after 5.0 seconds."
        }
        assert kubernetes_event_data(http_event, json_data) is not None

    util.wait_until(wait_for_events, 60, 3)


def test_agent_integration_sample_topology_events(host):
    expected_topology_events = [
        {
            "assertion": "find the URL timeout topology event",
            "event": {
               "category": "my_category",
               "name": "URL timeout",
               "tags": [],
               "data": "{\"another_thing\":1,\"big_black_hole\":\"here\",\"test\":{\"1\":\"test\"}}",
               "source_identifier": "source_identifier_value",
               "source": "source_value",
               "element_identifiers": [
                   "urn:host:/123"
               ],
               "source_links": [
                   {
                       "url": "http://localhost",
                       "name": "my_event_external_link"
                   }
               ],
               "type": "HTTP_TIMEOUT",
               "description": "Http request to http://localhost timed out after 5.0 seconds."
            }
        }
    ]
    util.assert_topology_events(host, "agent-integration-sample", "sts_topology_events", expected_topology_events)
