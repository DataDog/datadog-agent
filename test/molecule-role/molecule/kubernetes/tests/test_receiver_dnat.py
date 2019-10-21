import json
import os
import pytest
import re
import util

from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('kubernetes-cluster-agent')

kubeconfig_env = "KUBECONFIG=/home/ubuntu/deployment/aws-eks/tf-cluster/kubeconfig "


def _get_pod_ip(host, pod_name):
    pod_server_c = kubeconfig_env + "kubectl get pods/%s -o json" % pod_name
    pod_server_exec = host.check_output(pod_server_c)
    pod_server_data = json.loads(pod_server_exec)
    return pod_server_data["status"]["podIP"]


def _get_service_ip(host):
    service_c = kubeconfig_env + "kubectl get service/pod-service -o json"
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


@pytest.mark.last
def test_dnat(host, common_vars):
    url = "http://localhost:7070/api/topic/sts_topo_process_agents?offset=0&limit=1000"

    dnat_service_port = int(common_vars["dnat_service_port"])
    dnat_server_port = int(common_vars["dnat_server_port"])
    cluster_name = common_vars['cluster_name']

    def wait_for_components():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-topo-process-agents-dnat.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        pod_server_ip = _get_pod_ip(host, "pod-server")
        pod_service_ip = _get_service_ip(host)
        pod_client = _get_pod_ip(host, "pod-client")

        endpoint_match = re.compile("urn:endpoint:/.*:{}".format(pod_service_ip))
        endpoint = _find_component(
            json_data=json_data,
            type_name="endpoint",
            external_id_assert_fn=lambda v: endpoint_match.findall(v))
        assert json.loads(endpoint["data"])["ip"] == pod_service_ip
        endpoint_component_id = endpoint["externalId"]
        proc_to_proc_id_match = re.compile("TCP:/urn:process:/.*:.*->{}:{}".format(endpoint_component_id, dnat_service_port))
        proc_to_service_id_match = re.compile("TCP:/urn:process:/.*->urn:process:/.*:.*:{}:{}:{}".format(cluster_name, pod_server_ip, dnat_server_port))
        service_to_proc_id_match = re.compile("TCP:/{}:{}->urn:process:/.*:{}:{}:{}".format(endpoint_component_id, dnat_service_port, cluster_name, pod_server_ip, dnat_server_port))
        ""
        assert _relation_data(
            json_data=json_data,
            type_name="directional_connection",
            external_id_assert_fn=lambda v: proc_to_proc_id_match.findall(v))["outgoing"]["ip"] == pod_client
        assert _relation_data(
            json_data=json_data,
            type_name="directional_connection",
            external_id_assert_fn=lambda v: proc_to_service_id_match.findall(v))["outgoing"]["ip"] == pod_client
        assert _relation_data(
            json_data=json_data,
            type_name="directional_connection",
            external_id_assert_fn=lambda v: service_to_proc_id_match.findall(v))["incoming"]["ip"] == pod_server_ip

    util.wait_until(wait_for_components, 30, 3)
