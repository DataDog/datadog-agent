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


def _find_outgoing_connection_in_namespace(json_data, port, scope, origin, dest):
    """Find Connection as seen from the sending endpoint"""
    return next(connection for message in json_data["messages"]
                for connection in message["message"]["Connections"]["connections"]
                if connection["remoteEndpoint"]["endpoint"]["port"] == port and
                connection["remoteEndpoint"]["endpoint"]["ip"]["address"] == dest and
                connection["localEndpoint"]["endpoint"]["ip"]["address"] == origin and
                "scope" in connection["remoteEndpoint"] and
                connection["remoteEndpoint"]["scope"] == scope and
                "namespace" in connection["remoteEndpoint"] and "namespace" in connection["localEndpoint"] and
                connection["remoteEndpoint"]["namespace"] == connection["localEndpoint"]["namespace"] and
                connection["direction"] == "OUTGOING"
                )


def _find_incoming_connection_in_namespace(json_data, port, scope, origin, dest):
    """Find Connection as seen from the receiving endpoint"""
    return next(connection for message in json_data["messages"]
                for connection in message["message"]["Connections"]["connections"]
                if connection["localEndpoint"]["endpoint"]["port"] == port and
                connection["localEndpoint"]["endpoint"]["ip"]["address"] == dest and
                connection["remoteEndpoint"]["endpoint"]["ip"]["address"] == origin and
                "scope" in connection["localEndpoint"] and
                connection["localEndpoint"]["scope"] == scope and
                "namespace" in connection["remoteEndpoint"] and "namespace" in connection["localEndpoint"] and
                connection["remoteEndpoint"]["namespace"] == connection["localEndpoint"]["namespace"] and
                connection["direction"] == "INCOMING"
                )


def test_connection_network_namespaces_relations(host):
    url = "http://localhost:7070/api/topic/sts_correlate_endpoints?limit=1500"

    def wait_for_connection():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-correlate-endpoints.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        # assert that we find a outgoing localhost connection between 127.0.0.1 to 127.0.0.1 to port 9091 on
        # agent-connection-namespaces host within the same network namespace.
        outgoing_conn = _find_outgoing_connection_in_namespace(json_data, 9091, "agent-connection-namespaces", "127.0.0.1", "127.0.0.1")
        print(outgoing_conn)

        incoming_conn = _find_incoming_connection_in_namespace(json_data, 9091, "agent-connection-namespaces", "127.0.0.1", "127.0.0.1")
        print(incoming_conn)

        # assert that the connections are in the same namespace
        outgoing_local_namespace = outgoing_conn["localEndpoint"]["namespace"]
        outgoing_remote_namespace = outgoing_conn["remoteEndpoint"]["namespace"]
        incoming_local_namespace = incoming_conn["localEndpoint"]["namespace"]
        incoming_remote_namespace = incoming_conn["remoteEndpoint"]["namespace"]
        assert (
            outgoing_local_namespace == outgoing_remote_namespace and
            incoming_local_namespace == incoming_remote_namespace and
            incoming_remote_namespace == outgoing_local_namespace and
            incoming_local_namespace == outgoing_remote_namespace
        )

    util.wait_until(wait_for_connection, 30, 3)


def test_agent_http_metrics(host):
    url = "http://localhost:7070/api/topic/sts_multi_metrics?limit=1000"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-multi-metrics-http.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        def get_keys():
            return next(set(message["message"]["MultiMetric"]["values"].keys())
                        for message in json_data["messages"]
                        if message["message"]["MultiMetric"]["name"] == "connection metric" and
                        "code" in message["message"]["MultiMetric"]["tags"] and
                        message["message"]["MultiMetric"]["tags"]["code"] == "any"
                        )

        expected = {"http_requests_per_second", "http_response_time_seconds"}

        assert get_keys().pop() in expected
        assert get_keys() == expected

    util.wait_until(wait_for_metrics, 60, 3)
