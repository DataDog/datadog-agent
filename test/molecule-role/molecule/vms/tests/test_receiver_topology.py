import json
import os
import re
import util
from molecule.util import safe_load_file
from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('receiver_vm')


def _get_instance_config(instance_name):
    instance_config_dict = safe_load_file(os.environ['MOLECULE_INSTANCE_CONFIG'])
    return next(item for item in instance_config_dict if item['instance'] == instance_name)


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


def _relation_data(json_data, type_name, external_id_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyRelation" in p and \
            p["TopologyRelation"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyRelation"]["externalId"]):
            return json.loads(p["TopologyRelation"]["data"])
    return None


def test_hosts_processes(host):
    url = "http://localhost:7070/api/topic/sts_topo_process_agents?offset=0&limit=1000"

    def wait_for_components():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-topo-process-agents-hp.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        assert _component_data(json_data, "host", "urn:host:/agent-win", None)["system"]["os"]["name"] == "windows"
        assert _component_data(json_data, "host", "urn:host:/agent-fedora", None)["system"]["os"]["name"] == "linux"
        assert _component_data(json_data, "host", "urn:host:/agent-ubuntu", None)["system"]["os"]["name"] == "linux"
        assert _component_data(json_data, "host", "urn:host:/agent-centos", None)["system"]["os"]["name"] == "linux"
        assert _component_data(json_data, "process", "urn:process:/agent-fedora", "/opt/stackstate-agent/bin/agent/agent")["hostTags"] == ["os:linux"]
        assert _component_data(json_data, "process", "urn:process:/agent-ubuntu", "/opt/stackstate-agent/bin/agent/agent")["hostTags"] == ["os:linux"]
        assert _component_data(json_data, "process", "urn:process:/agent-centos", "/opt/stackstate-agent/bin/agent/agent")["hostTags"] == ["os:linux"]
        assert _component_data(json_data, "process", "urn:process:/agent-win", "\"C:\\Program Files\\StackState\\StackState Agent\\embedded\\agent.exe\"")["hostTags"] == ["os:windows"]

        # Assert that process filtering works correctly.
        # Process should be filtered unless it's a top resource using process
        def _component_filtered(type_name, external_id_prefix, command):
            _data = _component_data(json_data, type_name, external_id_prefix, command)
            if _data is not None:
                if "usage:top-cpu" in _data["tags"]:
                    return True
                if "usage:top-mem" in _data["tags"]:
                    return True
                if "usage:top-io-read" in _data["tags"]:
                    return True
                if "usage:top-io-write" in _data["tags"]:
                    return True
                # component was not filtered and is not a top resource consuming process
                return False
            # component was correctly filtered
            return True

        # fedora specific process filtering
        assert _component_filtered("process", "urn:process:/agent-fedora", "/usr/sbin/sshd")
        assert _component_filtered("process", "urn:process:/agent-fedora", "/usr/sbin/dhclient")
        assert _component_filtered("process", "urn:process:/agent-fedora", "/usr/lib/systemd/systemd-journald")
        assert _component_filtered("process", "urn:process:/agent-fedora", "/usr/bin/stress")
        # ubuntu specific process filtering
        assert _component_filtered("process", "urn:process:/agent-ubuntu", "/usr/sbin/sshd")
        assert _component_filtered("process", "urn:process:/agent-ubuntu", "/lib/systemd/systemd-journald")
        assert _component_filtered("process", "urn:process:/agent-ubuntu", "/sbin/agetty")
        assert _component_filtered("process", "urn:process:/agent-ubuntu", "/usr/bin/stress")
        # windows specific process filtering
        assert _component_filtered("process", "urn:process:/agent-win", "C:\\Windows\\system32\\svchost.exe")
        assert _component_filtered("process", "urn:process:/agent-win", "winlogon.exe")
        assert _component_filtered("process", "urn:process:/agent-win", "C:\\Windows\\system32\\wlms\\wlms.exe")
        # centos specific process filtering
        assert _component_filtered("process", "urn:process:/agent-centos", "/usr/sbin/sshd")
        assert _component_filtered("process", "urn:process:/agent-centos", "/sbin/init")
        assert _component_filtered("process", "urn:process:/agent-centos", "/sbin/agetty")
        assert _component_filtered("process", "urn:process:/agent-centos", "/usr/bin/stress")

    util.wait_until(wait_for_components, 30, 3)


def test_dnat(host, common_vars):
    url = "http://localhost:7070/api/topic/sts_topo_process_agents?offset=0&limit=1000"

    ubuntu_private_ip = _get_instance_config("agent-ubuntu")["private_address"]
    fedora_private_ip = _get_instance_config("agent-fedora")["private_address"]
    dnat_service_port = int(common_vars["dnat_service_port"])
    dnat_server_port = int(common_vars["dnat_server_port"])

    def wait_for_components():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-topo-process-agents-dnat.json", 'w') as f:
            json.dump(json_data, f, indent=4)

        service_component_id = "urn:service:/{}".format(ubuntu_private_ip)
        assert _component_data(
            json_data=json_data,
            type_name="dnat-service",
            external_id_prefix=service_component_id,
            command=None)["ip"] == ubuntu_private_ip

        proc_to_proc_id_match = re.compile("TCP:/urn:process:/agent-fedora:.*:.*->{}:{}".format(service_component_id, dnat_service_port))
        proc_to_service_id_match = re.compile("TCP:/urn:process:/agent-fedora:.*:.*->urn:process:/agent-ubuntu:.*:.*:{}:{}".format(ubuntu_private_ip, dnat_server_port))
        service_to_proc_id_match = re.compile("TCP:/{}:{}->urn:process:/agent-ubuntu:.*:.*:{}:{}".format(service_component_id, dnat_service_port, ubuntu_private_ip, dnat_server_port))
        assert _relation_data(
            json_data=json_data,
            type_name="directional_connection",
            external_id_assert_fn=lambda v: proc_to_proc_id_match.findall(v))["outgoing"]["ip"] == fedora_private_ip
        assert _relation_data(
            json_data=json_data,
            type_name="directional_connection",
            external_id_assert_fn=lambda v: proc_to_service_id_match.findall(v))["outgoing"]["ip"] == fedora_private_ip
        assert _relation_data(
            json_data=json_data,
            type_name="directional_connection",
            external_id_assert_fn=lambda v: service_to_proc_id_match.findall(v))["incoming"]["ip"] == ubuntu_private_ip

    util.wait_until(wait_for_components, 30, 3)
