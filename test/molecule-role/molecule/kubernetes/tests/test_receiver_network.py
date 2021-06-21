import json
import os
import re
import util

from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('kubernetes-cluster-agent')

kubeconfig_env = "KUBECONFIG=/home/ubuntu/deployment/aws-eks/tf-cluster/kubeconfig "


def _get_pod_ip(host, namespace, pod_name):
    pod_server_c = kubeconfig_env + "kubectl get pods/{0} -o json --namespace={1}".format(pod_name, namespace)
    pod_server_exec = host.check_output(pod_server_c)
    pod_server_data = json.loads(pod_server_exec)
    return pod_server_data["status"]["podIP"]


def _get_service_ip(host, namespace):
    service_c = kubeconfig_env + "kubectl get service/pod-service -o json --namespace={0}".format(namespace)
    pod_service_exec = host.check_output(service_c)
    pod_service_data = json.loads(pod_service_exec)
    return pod_service_data["spec"]["clusterIP"]


def _component_data(json_data, type_name, external_id_prefix, command):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyComponent" in p and \
            p["TopologyComponent"]["typeName"] == type_name and \
                p["TopologyComponent"]["externalId"].startswith(external_id_prefix):
            component_data = json.loads(p["TopologyComponent"]["data"])
            if command:
                if "args" in component_data["command"]:
                    if component_data["command"]["args"][0] == command:
                        return component_data
            else:
                return component_data
    return None


def _find_component(json_data, type_name, external_id_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyComponent" in p and \
            p["TopologyComponent"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyComponent"]["externalId"]):
            return p["TopologyComponent"]
    return None


def _relation_data(json_data, type_name, external_id_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyRelation" in p and \
            p["TopologyRelation"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyRelation"]["externalId"]):
            return json.loads(p["TopologyRelation"]["data"])
    return None


def _find_process_by_command_args(json_data, type_name, cmd_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyComponent" in p and \
            p["TopologyComponent"]["typeName"] == type_name and \
                "data" in p["TopologyComponent"]:
            component_data = json.loads(p["TopologyComponent"]["data"])
            if "args" in component_data["command"] and cmd_assert_fn(' '.join(component_data["command"]["args"])):
                return component_data
    return None


def test_dnat(host, ansible_var):
    url = "http://localhost:7070/api/topic/sts_topo_process_agents?limit=1000"
    correlate_url = "http://localhost:7070/api/topic/sts_correlate_endpoints?limit=100"

    dnat_service_port = int(ansible_var("dnat_service_port"))
    namespace = ansible_var("namespace")

    def wait_for_components():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-topo-process-agents-dnat.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        # This is here for debugging
        correlate_data = host.check_output("curl \"%s\"" % correlate_url)
        correlate_json_data = json.loads(correlate_data)
        with open("./topic-topo-process-agents-dnat-correlate.json", 'w') as f:
            json.dump(correlate_json_data, f, indent=4)

        pod_service_ip = _get_service_ip(host, namespace)
        pod_client = _get_pod_ip(host, namespace, "pod-client")

        endpoint_match = re.compile("urn:endpoint:/.*:{}".format(pod_service_ip))
        endpoint = _find_component(
            json_data=json_data,
            type_name="endpoint",
            external_id_assert_fn=lambda v: endpoint_match.findall(v))
        assert json.loads(endpoint["data"])["ip"] == pod_service_ip
        endpoint_component_id = endpoint["externalId"]
        proc_to_service_id_match = re.compile("TCP:/urn:process:/.*:.*->{}:{}".format(endpoint_component_id, dnat_service_port))

        assert _relation_data(
            json_data=json_data,
            type_name="directional_connection",
            external_id_assert_fn=lambda v: proc_to_service_id_match.findall(v))["outgoing"]["ip"] == pod_client

    util.wait_until(wait_for_components, 600, 3)


def test_pod_container_to_container(host, ansible_var):
    url = "http://localhost:7070/api/topic/sts_topo_process_agents?limit=1000"

    server_port = int(ansible_var("container_to_container_server_port"))
    cluster_name = ansible_var("cluster_name")

    def wait_for_components():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-topo-process-agents-container-container.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        server_process_match = re.compile("nc -l -p {}".format(server_port))
        server_process = _find_process_by_command_args(
            json_data=json_data,
            type_name="process",
            cmd_assert_fn=lambda v: server_process_match.findall(v)
        )
        assert server_process is not None
        server_process_create_time = server_process["createTime"]
        server_process_pid = server_process["pid"]
        server_host = server_process["host"]

        request_process_match = re.compile("nc localhost {}".format(server_port))
        request_process = _find_process_by_command_args(
            json_data=json_data,
            type_name="process",
            cmd_assert_fn=lambda v: request_process_match.findall(v)
        )
        assert request_process is not None
        request_process_create_time = request_process["createTime"]
        request_process_pid = request_process["pid"]
        request_host = request_process["host"]

        request_process_to_server_relation_match = "TCP:/urn:process:/{}:{}:{}->urn:process:/{}:{}:{}:{}:{}:{}:.*:127.0.0.1:{}".format(
            request_host, request_process_pid, request_process_create_time,
            server_host, server_process_pid, server_process_create_time,
            server_host, cluster_name, server_host, server_port
        )

        assert _relation_data(
                json_data=json_data,
                type_name="directional_connection",
                external_id_assert_fn=lambda v: re.compile(request_process_to_server_relation_match).findall(v)
            ) is not None

    util.wait_until(wait_for_components, 600, 3)


def test_headless_pod_to_pod(host, ansible_var):
    url = "http://localhost:7070/api/topic/sts_topo_process_agents?limit=1000"

    # Server and service port are equal
    server_port = int(ansible_var("headless_service_port"))
    cluster_name = ansible_var("cluster_name")

    def wait_for_components():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-topo-process-agents-headless.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        server_process_match = re.compile("ncat -vv --broker --listen -p {}".format(server_port))
        server_process = _find_process_by_command_args(
            json_data=json_data,
            type_name="process",
            cmd_assert_fn=lambda v: server_process_match.findall(v)
        )
        assert server_process is not None
        server_process_create_time = server_process["createTime"]
        server_process_pid = server_process["pid"]
        server_host = server_process["host"]

        request_process_match = re.compile("nc -vv headless-service {}".format(server_port))
        request_process = _find_process_by_command_args(
            json_data=json_data,
            type_name="process",
            cmd_assert_fn=lambda v: request_process_match.findall(v)
        )
        assert request_process is not None
        request_process_create_time = request_process["createTime"]
        request_process_pid = request_process["pid"]
        request_host = request_process["host"]

        request_process_to_server_relation_match = re.compile(
            "TCP:/urn:process:/{}:{}:{}->urn:process:/{}:{}:{}:{}:.*:{}"
            .format(request_host, request_process_pid, request_process_create_time,
                    server_host, server_process_pid, server_process_create_time,
                    cluster_name, server_port)
        )

        assert _relation_data(
                json_data=json_data,
                type_name="directional_connection",
                external_id_assert_fn=lambda v: request_process_to_server_relation_match.findall(v)
            ) is not None

    util.wait_until(wait_for_components, 600, 3)
