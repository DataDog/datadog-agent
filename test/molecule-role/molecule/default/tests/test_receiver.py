import os
import json
import util
from collections import defaultdict
from molecule.util import safe_load_file
from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('receiver_vm')


def test_etc_docker_directory(host):
    f = host.file('/etc/docker/')
    assert f.is_directory


def test_docker_compose_file(host):
    f = host.file('/home/ubuntu/docker-compose.yml')
    assert f.is_file


def test_receiver_ok(host):
    c = "curl -s -o /dev/null -w \"%{http_code}\" http://localhost:7077/health"
    assert host.check_output(c) == "200"


def test_generic_events(host):
    url = "http://localhost:7070/api/topic/sts_generic_events?offset=0&limit=80"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-generic-events.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        events = defaultdict(set)
        for message in json_data["messages"]:
            events[message["message"]["GenericEvent"]["host"]].add(message["message"]["GenericEvent"]["name"])

        print(events)
        assert events["agent-ubuntu"] == {"System.Agent Startup", "processStateEvent"}
        assert events["agent-fedora"] == {"System.Agent Startup", "processStateEvent"}
        assert events["agent-centos"] == {"System.Agent Startup", "processStateEvent"}
        assert events["agent-win"] == {"System.Agent Startup"}

    util.wait_until(wait_for_metrics, 30, 3)


def _get_instance_config(instance_name):
    instance_config_dict = safe_load_file(os.environ['MOLECULE_INSTANCE_CONFIG'])
    return next(item for item in instance_config_dict if item['instance'] == instance_name)


def _find_outgoing_connection(json_data, port, origin, dest):
    """Find Connection as seen from the sending endpoint"""
    return next(connection for message in json_data["messages"]
                for connection in message["message"]["Connections"]["connections"]
                if connection["remoteEndpoint"]["endpoint"]["port"] == port and
                connection["remoteEndpoint"]["endpoint"]["ip"]["address"] == dest and
                connection["localEndpoint"]["endpoint"]["ip"]["address"] == origin
                )


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


def _find_incoming_connection(json_data, port, origin, dest):
    """Find Connection as seen from the receiving endpoint"""
    return next(connection for message in json_data["messages"]
                for connection in message["message"]["Connections"]["connections"]
                if connection["localEndpoint"]["endpoint"]["port"] == port and
                connection["localEndpoint"]["endpoint"]["ip"]["address"] == dest and
                connection["remoteEndpoint"]["endpoint"]["ip"]["address"] == origin
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


def test_created_connection_after_start_with_metrics(host, common_vars):
    url = "http://localhost:7070/api/topic/sts_correlate_endpoints?limit=1000"

    fedora_conn_port = int(common_vars["connection_port_after_start_fedora"])
    windows_conn_port = int(common_vars["connection_port_after_start_windows"])

    ubuntu_private_ip = _get_instance_config("agent-ubuntu")["private_address"]
    print("ubuntu private: {}".format(ubuntu_private_ip))
    fedora_private_ip = _get_instance_config("agent-fedora")["private_address"]
    print("fedora private: {}".format(fedora_private_ip))
    windows_private_ip = _get_instance_config("agent-win")["private_address"]
    print("windows private: {}".format(windows_private_ip))

    def wait_for_connection():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-correlate-endpoint-after.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        outgoing_conn = _find_outgoing_connection(json_data, fedora_conn_port, fedora_private_ip, ubuntu_private_ip)
        print(outgoing_conn)
        assert outgoing_conn["direction"] == "OUTGOING"
        assert outgoing_conn["connectionType"] == "TCP"
        assert outgoing_conn["bytesSentPerSecond"] > 10.0
        assert outgoing_conn["bytesReceivedPerSecond"] == 0.0

        incoming_conn = _find_incoming_connection(json_data, fedora_conn_port, fedora_private_ip, ubuntu_private_ip)
        print(incoming_conn)
        assert incoming_conn["direction"] == "INCOMING"
        assert incoming_conn["connectionType"] == "TCP"
        assert incoming_conn["bytesSentPerSecond"] == 0.0
        assert incoming_conn["bytesReceivedPerSecond"] > 10.0

        outgoing_conn = _find_outgoing_connection(json_data, windows_conn_port, windows_private_ip, ubuntu_private_ip)
        print(outgoing_conn)
        assert outgoing_conn["direction"] == "OUTGOING"
        assert outgoing_conn["connectionType"] == "TCP"
        assert outgoing_conn["bytesSentPerSecond"] == 0.0       # We don't collect metrics on Windows
        assert outgoing_conn["bytesReceivedPerSecond"] == 0.0

        incoming_conn = _find_incoming_connection(json_data, windows_conn_port, windows_private_ip, ubuntu_private_ip)
        print(incoming_conn)
        assert incoming_conn["direction"] == "INCOMING"
        assert incoming_conn["connectionType"] == "TCP"
        assert incoming_conn["bytesSentPerSecond"] == 0.0
        assert incoming_conn["bytesReceivedPerSecond"] == 0.0   # We don't send data from Windows

    util.wait_until(wait_for_connection, 30, 3)


def test_created_connection_before_start(host, common_vars):
    url = "http://localhost:7070/api/topic/sts_correlate_endpoints?limit=1000"

    fedora_conn_port = int(common_vars["connection_port_before_start_fedora"])
    windows_conn_port = int(common_vars["connection_port_before_start_windows"])

    ubuntu_private_ip = _get_instance_config("agent-ubuntu")["private_address"]
    print("ubuntu private: {}".format(ubuntu_private_ip))
    fedora_private_ip = _get_instance_config("agent-fedora")["private_address"]
    print("fedora private: {}".format(fedora_private_ip))
    windows_private_ip = _get_instance_config("agent-win")["private_address"]
    print("windows private: {}".format(windows_private_ip))

    def wait_for_connection():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-correlate-endpoint-before.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        outgoing_conn = _find_outgoing_connection(json_data, fedora_conn_port, fedora_private_ip, ubuntu_private_ip)
        print(outgoing_conn)
        assert outgoing_conn["direction"] == "NONE"          # Outgoing gets no direction from Linux /proc scanning
        assert outgoing_conn["connectionType"] == "TCP"

        incoming_conn = _find_incoming_connection(json_data, fedora_conn_port, fedora_private_ip, ubuntu_private_ip)
        print(incoming_conn)
        assert incoming_conn["direction"] == "INCOMING"
        assert incoming_conn["connectionType"] == "TCP"

        outgoing_conn = _find_outgoing_connection(json_data, windows_conn_port, windows_private_ip, ubuntu_private_ip)
        print(outgoing_conn)
        assert outgoing_conn["direction"] == "OUTGOING"
        assert outgoing_conn["connectionType"] == "TCP"

        incoming_conn = _find_incoming_connection(json_data, windows_conn_port, windows_private_ip, ubuntu_private_ip)
        print(incoming_conn)
        assert incoming_conn["direction"] == "INCOMING"
        assert incoming_conn["connectionType"] == "TCP"

    util.wait_until(wait_for_connection, 30, 3)


def test_host_metrics(host):
    url = "http://localhost:7070/api/topic/sts_metrics?limit=2000"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-metrics.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        metrics = {}
        for message in json_data["messages"]:
            metric = message["message"]["Metric"]

            m_name = metric["name"]
            m_host = metric["host"]

            if m_name not in metrics:
                metrics[m_name] = {}
            if m_host not in metrics[m_name]:
                metrics[m_name][m_host] = []

            values = [value["value"] for value in metric["value"]]
            metrics[m_name][m_host] += values

        # These values are based on an ec2 micro instance for ubuntu and fedora
        # and small instance for windows
        # (as created by molecule.yml)

        # Same metrics we check in the backend e2e tests
        # https://stackvista.githost.io/StackVista/StackState/blob/master/stackstate-pm-test/src/test/scala/com/stackstate/it/e2e/ProcessAgentIntegrationE2E.scala#L17

        # No swap in these tests, we still wanna know whether it is reported
        def assert_metric(name, ubuntu_predicate, fedora_predicate, win_predicate):
            if ubuntu_predicate:
                for uv in metrics[name]["agent-ubuntu"]:
                    assert ubuntu_predicate(uv)
            if fedora_predicate:
                for fv in metrics[name]["agent-fedora"]:
                    assert fedora_predicate(fv)
            if win_predicate:
                for wv in metrics[name]["agent-win"]:
                    assert win_predicate(wv)

        assert_metric("system.uptime", lambda v: v > 1.0, lambda v: v > 1.0, lambda v: v > 1.0)

        assert_metric("system.swap.total", lambda v: v == 0, lambda v: v == 0, lambda v: v > 2000)
        assert_metric("system.swap.pct_free", lambda v: v == 1.0, lambda v: v == 1.0, lambda v: v == 1.0)

        # Memory
        assert_metric("system.mem.total", lambda v: v > 900.0, lambda v: v > 900.0, lambda v: v > 2000.0)
        assert_metric("system.mem.usable", lambda v: 1000.0 > v > 300.0, lambda v: 1000.0 > v > 300.0, lambda v: 1500.0 > v > 400.0)
        assert_metric("system.mem.pct_usable", lambda v: 1.0 > v > 0.3, lambda v: 1.0 > v > 0.3, lambda v: 1.0 > v > 0.2)

        # Load - only linux
        assert_metric("system.load.norm.1", lambda v: v >= 0.0, lambda v: v >= 0.0, None)

        # CPU
        assert_metric("system.cpu.idle", lambda v: v > 0.0, lambda v: v > 0.0, lambda v: v > 0.0)
        assert_metric("system.cpu.iowait", lambda v: v >= 0.0, lambda v: v >= 0.0, lambda v: v >= 0.0)
        assert_metric("system.cpu.system", lambda v: v > 0.0, lambda v: v > 0.0, lambda v: v > 0.0)
        assert_metric("system.cpu.user", lambda v: v >= 0.0, lambda v: v >= 0.0, lambda v: v >= 0.0)

        # Inodes
        assert_metric("system.fs.file_handles.in_use", lambda v: v > 0.0, lambda v: v > 0.0, lambda v: v > 0.0)
        # only linux
        assert_metric("system.fs.file_handles.max", lambda v: v > 10000.0, lambda v: v > 10000.0, None)

        # Agent metrics
        assert_metric("stackstate.agent.running", lambda v: v == 1.0, lambda v: v == 1.0, lambda v: v == 1.0)
        assert_metric("stackstate.process.agent", lambda v: v == 1.0, lambda v: v == 1.0, lambda v: v == 1.0)
        assert_metric("stackstate.process.processes.host_count", lambda v: v > 1.0, lambda v: v > 1.0, lambda v: v > 1.0)
        assert_metric("stackstate.process.containers.host_count", lambda v: v == 0.0, lambda v: v == 0.0, lambda v: v == 0.0)

    util.wait_until(wait_for_metrics, 30, 3)


def test_process_metrics(host):
    url = "http://localhost:7070/api/topic/sts_multi_metrics?limit=1000"

    def wait_for_metrics():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-multi-metrics.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        def get_keys(m_host):
            return next(set(message["message"]["MultiMetric"]["values"].keys())
                        for message in json_data["messages"]
                        if message["message"]["MultiMetric"]["name"] == "processMetrics" and
                        message["message"]["MultiMetric"]["host"] == m_host
                        )

        # Same metrics we check in the backend e2e tests
        # https://stackvista.githost.io/StackVista/StackState/blob/master/stackstate-pm-test/src/test/scala/com/stackstate/it/e2e/ProcessAgentIntegrationE2E.scala#L17

        expected = {"cpu_nice", "cpu_userPct", "cpu_userTime", "cpu_systemPct", "cpu_numThreads", "io_writeRate",
                    "io_writeBytesRate", "cpu_totalPct", "voluntaryCtxSwitches", "mem_dirty", "involuntaryCtxSwitches",
                    "io_readRate", "openFdCount", "mem_shared", "cpu_systemTime", "io_readBytesRate", "mem_data",
                    "mem_vms", "mem_lib", "mem_text", "mem_swap", "mem_rss"}

        assert get_keys("agent-ubuntu") == expected
        assert get_keys("agent-fedora") == expected
        assert get_keys("agent-centos") == expected
        assert get_keys("agent-win") == expected

    util.wait_until(wait_for_metrics, 30, 3)


def test_connection_network_namespaces_relations(host):
    url = "http://localhost:7070/api/topic/sts_correlate_endpoints?limit=1500"

    def wait_for_connection():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-correlate-endpoint-netns.json", 'w') as f:
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
