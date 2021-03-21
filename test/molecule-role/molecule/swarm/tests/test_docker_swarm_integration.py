import os
import json
import re

import testinfra.utils.ansible_runner

import util

testinfra_hosts = testinfra.utils.ansible_runner.AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('agent-swarm-master')


def test_hosts_file(host):
    f = host.file('/etc/hosts')

    assert f.exists
    assert f.user == 'root'
    assert f.group == 'root'


def test_docker_swarm_metrics(host):
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


def test_docker_swarm_topology(host):

    def assert_topology():
        topo_url = "http://localhost:7070/api/topic/sts_topo_docker-swarm_agents?limit=1500"
        data = host.check_output('curl "{}"'.format(topo_url))
        json_data = json.loads(data)
        with open("./topic-agent-integration-sample-topo-agent-integrations.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        components = [
            {
                "assertion": "Should find the nginx swarm service component",
                "type": "swarm-service",
                "external_id": lambda e_id: re.compile(
                    r"urn:swarm-service:/.*\..*\..*\..*").findall(e_id),
                "data": lambda d: (
                    d["name"] == "nginx" and
                    d["image"].startsWith("nginx:latest@") and
                    "spec" in d and
                    "global" in d["spec"]["Mode"]
                )
            },
            {
                "assertion": "Should find the agent swarm service component",
                "type": "swarm-service",
                "external_id": lambda e_id: re.compile(
                    r"urn:swarm-service:/.*\..*\..*\..*").findall(e_id),
                "data": lambda d: (
                    d["name"] == "agent_stackstate-agent" and
                    d["image"].startsWith("stackstate/stackstate-agent-2-test:{}@".format(os.environ['STACKSTATE_BRANCH'])) and
                    "spec" in d and
                    "Replicated" in d["spec"]["Mode"] and
                    d["spec"]["Mode"]["Replicated"]["Replicas"] == 1
                )
            },
            {
                "assertion": "Should find the receiver service component",
                "type": "swarm-service",
                "external_id": lambda e_id: re.compile(
                    r"urn:swarm-service:/.*\..*\..*\..*").findall(e_id),
                "data": lambda d: (
                    d["name"] == "agent_receiver" and
                    d["image"].startsWith("stackstate/stackstate-receiver:{}@".format(os.environ['STACKSTATE_BRANCH'])) and
                    "spec" in d and
                    "Replicated" in d["spec"]["Mode"] and
                    d["spec"]["Mode"]["Replicated"]["Replicas"] == 1
                )
            },
            {
                "assertion": "Should find the topic-api swarm service component",
                "type": "swarm-service",
                "external_id": lambda e_id: re.compile(
                    r"urn:swarm-service:/.*\..*\..*\..*").findall(e_id),
                "data": lambda d: (
                    d["name"] == "agent_topic-api" and
                    d["image"].startsWith("stackstate/stackstate-topic-api:{}@".format(os.environ['STACKSTATE_BRANCH'])) and
                    "spec" in d and
                    "Replicated" in d["spec"]["Mode"] and
                    d["spec"]["Mode"]["Replicated"]["Replicas"] == 1
                )
            },
            {
                "assertion": "Should find the kafka swarm service component",
                "type": "swarm-service",
                "external_id": lambda e_id: re.compile(
                    r"urn:swarm-service:/.*\..*\..*\..*").findall(e_id),
                "data": lambda d: (
                    d["name"] == "agent_kafka" and
                    d["image"].startsWith("wurstmeister/kafka:2.12-2.3.1@") and
                    "spec" in d and
                    "Replicated" in d["spec"]["Mode"] and
                    d["spec"]["Mode"]["Replicated"]["Replicas"] == 1
                )
            },
            {
                "assertion": "Should find the zookeeper swarm service component",
                "type": "swarm-service",
                "external_id": lambda e_id: re.compile(
                    r"urn:swarm-service:/.*\..*\..*\..*").findall(e_id),
                "data": lambda d: (
                    d["name"] == "agent_zookeeper" and
                    d["image"].startsWith("wurstmeister/zookeeper:latest@") and
                    "spec" in d and
                    "Replicated" in d["spec"]["Mode"] and
                    d["spec"]["Mode"]["Replicated"]["Replicas"] == 1
                )
            }
        ]
        for c in components:
            print("Running assertion for: " + c["assertion"])
            assert util.component_data(
                json_data=json_data,
                type_name=c["type"],
                external_id_assert_fn=c["external_id"],
                data_assert_fn=c["data"],
            ) is not None

    util.wait_until(assert_topology, 30, 3)
