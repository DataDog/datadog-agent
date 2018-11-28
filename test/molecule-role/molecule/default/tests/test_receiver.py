import os
import json
import util
import testinfra.utils.ansible_runner

testinfra_hosts = testinfra.utils.ansible_runner.AnsibleRunner(
    os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('receiver_vm')


def test_etc_docker_directory(host):
    f = host.file('/etc/docker/')
    assert f.is_directory


def test_docker_compose_file(host):
    f = host.file('/home/ubuntu/docker-compose.yml')
    assert f.is_file


def test_receiver_ok(host):
    c = "curl -s -o /dev/null -w \"%{http_code}\" http://localhost:7077/health"
    assert host.check_output(c) == "200"


def test_created_connection_with_metrics(host, Ansible):
    url = "http://localhost:7070/api/topic/sts_correlate_endpoints?limit=1000"
    # FIXME: Maybe there is a more direct way to get this variable
    conn_port = int(Ansible("include_vars", "./common_vars.yml")
                    ["ansible_facts"]["test_connection_port"])

    def wait_for_connection():
        data = host.check_output("curl %s" % url)
        json_data = json.loads(data)
        outgoing_conn = next(connection for message in json_data["messages"]
                             for connection
                             in message["message"]
                             ["Connections"]["connections"]
                             if connection["remoteEndpoint"]
                             ["endpoint"]["port"] == conn_port
                             )
        print outgoing_conn
        assert outgoing_conn["direction"] == "OUTGOING"
        assert outgoing_conn["connectionType"] == "TCP"
        assert outgoing_conn["bytesSentPerSecond"] > 10.0
        assert outgoing_conn["bytesReceivedPerSecond"] == 0.0
        incoming_conn = next(connection for message in json_data["messages"]
                             for connection
                             in message["message"]
                             ["Connections"]["connections"]
                             if connection["localEndpoint"]
                             ["endpoint"]["port"] == conn_port
                             )
        print incoming_conn
        assert incoming_conn["direction"] == "INCOMING"
        assert incoming_conn["connectionType"] == "TCP"
        assert incoming_conn["bytesSentPerSecond"] == 0.0
        assert incoming_conn["bytesReceivedPerSecond"] > 10.0

    util.wait_until(wait_for_connection, 30, 3)
