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


def test_created_connection(host, Ansible):
    url = "http://localhost:7070/api/topic/sts_correlate_endpoints?limit=1000"
    # FIXME: Maybe there is a more direct way to get this variable
    conn_port = int(Ansible("include_vars", "./common_vars.yml")
                    ["ansible_facts"]["test_connection_port"])

    def wait_for_connection():
        data = host.check_output("curl %s" % url)
        json_data = json.loads(data)
        outgoing = next(record
                        for record
                        in json_data["messages"]
                        if record["message"]["Connection"]
                        ["remoteEndpoint"]["endpoint"]["port"] == conn_port
                        )
        outgoing_conn = outgoing["message"]["Connection"]
        print outgoing_conn
        assert outgoing_conn["direction"] == "OUTGOING"
        assert outgoing_conn["connectionType"] == "TCP"
        incoming = next(record
                        for record
                        in json_data["messages"]
                        if record["message"]["Connection"]
                        ["localEndpoint"]["endpoint"]["port"] == conn_port
                        )
        incoming_conn = incoming["message"]["Connection"]
        print incoming_conn
        assert incoming_conn["direction"] == "INCOMING"
        assert incoming_conn["connectionType"] == "TCP"

    util.wait_until(wait_for_connection, 30, 3)
