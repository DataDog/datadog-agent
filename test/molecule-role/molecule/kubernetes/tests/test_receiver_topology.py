import json
import os
import re

import util

from testinfra.utils.ansible_runner import AnsibleRunner

testinfra_hosts = AnsibleRunner(os.environ['MOLECULE_INVENTORY_FILE']).get_hosts('kubernetes-cluster-agent')


def _component_data(json_data, type_name, external_id_assert_fn, cluster_name, identifiers_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyComponent" in p and \
            p["TopologyComponent"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyComponent"]["externalId"]):
            component_data = json.loads(p["TopologyComponent"]["data"])
            if "cluster-name" in component_data["tags"]:
                if component_data["tags"]["cluster-name"] == cluster_name and \
                        identifiers_assert_fn(component_data["identifiers"]):
                    return component_data
    return None


def _relation_data(json_data, type_name, external_id_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyRelation" in p and \
            p["TopologyRelation"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyRelation"]["externalId"]):
            return p["TopologyRelation"]["sourceId"]
    return None


def _find_component(json_data, type_name, external_id_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyComponent" in p and \
            p["TopologyComponent"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyComponent"]["externalId"]):
            return p["TopologyComponent"]
    return None


def test_agent_base_topology(host, common_vars):
    cluster_name = common_vars['cluster_name']
    namespace = "default"
    topic = "sts_topo_kubernetes_%s" % cluster_name
    url = "http://localhost:7070/api/topic/%s?offset=0&limit=1000" % topic

    def wait_for_components():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-" + topic + ".json", 'w') as f:
            json.dump(json_data, f, indent=4)

        # TODO make sure we identify the 2 different ec2 instances using i-*
        # 2 nodes
        assert _component_data(
            json_data=json_data,
            type_name="node",
            external_id_assert_fn=lambda eid: eid.startswith("urn:/kubernetes:%s:node:" % cluster_name),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:ip:/%s:" % cluster_name))
        )
        # 2 agent pods on each node, each pod 1 container
        assert _component_data(
            json_data=json_data,
            type_name="pod",
            external_id_assert_fn=lambda eid: eid.startswith("urn:/kubernetes:%s:pod:stackstate-agent-" % cluster_name),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:ip:/%s:" % cluster_name))
        )
        node_agent_container_match = re.compile("urn:/kubernetes:%s:pod:stackstate-agent-.*:container:stackstate-agent" % cluster_name)
        assert _component_data(
            json_data=json_data,
            type_name="container",
            external_id_assert_fn=lambda eid: node_agent_container_match.findall(eid),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:container:/i-"))  # TODO ec2 i-*
        )
        # 1 cluster agent pod with 1 container
        assert _component_data(
            json_data=json_data,
            type_name="pod",
            external_id_assert_fn=lambda eid: eid.startswith("urn:/kubernetes:%s:pod:stackstate-cluster-agent-" % cluster_name),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:ip:/%s:" % cluster_name))
        )
        cluster_agent_container_match = re.compile("urn:/kubernetes:%s:pod:stackstate-cluster-agent-.*:container:stackstate-cluster-agent" % cluster_name)
        assert _component_data(
            json_data=json_data,
            type_name="container",
            external_id_assert_fn=lambda eid: cluster_agent_container_match.findall(eid),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:container:/i-"))  # TODO ec2 i-*
        )
        # 2 service, one for each agent
        assert _component_data(
            json_data=json_data,
            type_name="service",
            external_id_assert_fn=lambda eid: eid.startswith("urn:/kubernetes:%s:service:%s:stackstate-agent" % (cluster_name, namespace)),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:endpoint:/%s:" % cluster_name))
        )
        assert _component_data(
            json_data=json_data,
            type_name="service",
            external_id_assert_fn=lambda eid: eid.startswith("urn:/kubernetes:%s:service:%s:stackstate-cluster-agent" % (cluster_name, namespace)),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:endpoint:/%s:" % cluster_name))
        )
        # 1 service, pod-service for dnat
        assert _component_data(
            json_data=json_data,
            type_name="service",
            external_id_assert_fn=lambda eid: eid.startswith("urn:/kubernetes:%s:service:%s:pod-service" % (cluster_name, namespace)),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:endpoint:/%s:" % cluster_name))
        )
        # 1 config map
        configmap_match = re.compile("urn:/kubernetes:{}:configmap:kube-system:aws-auth".format(cluster_name))
        assert _find_component(
            json_data=json_data,
            type_name="configmap",
            external_id_assert_fn=lambda v: configmap_match.findall(v)
        )
        # 1 replicaset cluster-agent
        replicaset_match = re.compile("urn:/kubernetes:{}:replicaset:stackstate-cluster-agent-.*".format(cluster_name))
        assert _find_component(
            json_data=json_data,
            type_name="replicaset",
            external_id_assert_fn=lambda v: replicaset_match.findall(v)
        )
        # 1 deployment cluster-agent
        deployment_match = re.compile("urn:/kubernetes:{}:deployment:stackstate-cluster-agent".format(cluster_name))
        assert _find_component(
            json_data=json_data,
            type_name="deployment",
            external_id_assert_fn=lambda v: deployment_match.findall(v)
        )
        # 1 daemonset node-agent
        daemonset_match = re.compile("urn:/kubernetes:{}:daemonset:stackstate-agent".format(cluster_name))
        assert _find_component(
            json_data=json_data,
            type_name="daemonset",
            external_id_assert_fn=lambda v: daemonset_match.findall(v)
        )

        # Pod -> Node (scheduled on)
        # stackstate-agent pods is scheduled_on a node (2 times)
        node_agent_pod_scheduled_match = re.compile("urn:/kubernetes:%s:pod:stackstate-agent-.*->urn:/kubernetes:%s:node:ip-.*" % (cluster_name, cluster_name))
        assert _relation_data(
            json_data=json_data,
            type_name="scheduled_on",
            external_id_assert_fn=lambda eid: node_agent_pod_scheduled_match.findall(eid)
        ).startswith("urn:/kubernetes:%s:pod:stackstate-agent-" % cluster_name)
        # stackstate-cluster-agent pod is scheduled_on a node (1 time)
        cluster_agent_pod_scheduled_match = re.compile("urn:/kubernetes:%s:pod:stackstate-cluster-agent-.*->urn:/kubernetes:%s:node:ip-.*" % (cluster_name, cluster_name))
        assert _relation_data(
            json_data=json_data,
            type_name="scheduled_on",
            external_id_assert_fn=lambda eid: cluster_agent_pod_scheduled_match.findall(eid)
        ).startswith("urn:/kubernetes:%s:pod:stackstate-cluster-agent" % cluster_name)
        # Container -> Pod (enclosed in)
        # stackstate-agent containers are enclosed_in a pod (2 times)
        node_agent_container_enclosed_match = re.compile(
            "urn:/kubernetes:%s:pod:stackstate-agent-.*:container:stackstate-agent->urn:/kubernetes:%s:pod:stackstate-agent-.*"
            % (cluster_name, cluster_name))
        node_enclosed_source_id = _relation_data(
            json_data=json_data,
            type_name="enclosed_in",
            external_id_assert_fn=lambda eid: node_agent_container_enclosed_match.findall(eid)
        )
        assert re.match(
            "urn:/kubernetes:%s:pod:stackstate-agent-.*:container:stackstate-agent"
            % cluster_name, node_enclosed_source_id)
        # stackstate-cluster-agent container are enclosed_in a pod (1 time)
        cluster_agent_container_enclosed_match = re.compile(
            "urn:/kubernetes:%s:pod:stackstate-cluster-agent-.*:container:stackstate-cluster-agent->urn:/kubernetes:%s:pod:stackstate-cluster-agent-.*"
            % (cluster_name, cluster_name))
        node_enclosed_source_id = _relation_data(
            json_data=json_data,
            type_name="enclosed_in",
            external_id_assert_fn=lambda eid: cluster_agent_container_enclosed_match.findall(eid)
        )
        assert re.match("urn:/kubernetes:%s:pod:stackstate-cluster-agent-.*:container:stackstate-cluster-agent" % cluster_name, node_enclosed_source_id)
        # Pod -> Service (exposes)
        # stackstate-agent exposes stackstate-agent pods (2 times)
        node_agent_service_match = re.compile("urn:/kubernetes:%s:service:%s:stackstate-agent->urn:/kubernetes:%s:pod:stackstate-agent-.*" % (cluster_name, namespace, cluster_name))
        assert _relation_data(
            json_data=json_data,
            type_name="exposes",
            external_id_assert_fn=lambda eid:  node_agent_service_match.findall(eid)
        ).startswith("urn:/kubernetes:%s:service:%s:stackstate-agent" % (cluster_name, namespace))
        # stackstate-cluster-agent exposes stackstate-cluster-agent pod (1 time)
        cluster_agent_service_match = re.compile("urn:/kubernetes:%s:service:%s:stackstate-cluster-agent->urn:/kubernetes:%s:pod:stackstate-cluster-agent-.*" % (cluster_name, namespace, cluster_name))
        assert _relation_data(
            json_data=json_data,
            type_name="exposes",
            external_id_assert_fn=lambda eid:  cluster_agent_service_match.findall(eid)
        ).startswith("urn:/kubernetes:%s:service:%s:stackstate-cluster-agent" % (cluster_name, namespace))
        # pod-service exposes pod-server (1 time)
        pod_service_match = re.compile("urn:/kubernetes:%s:service:%s:pod-service->urn:/kubernetes:%s:pod:pod-server" % (cluster_name, namespace, cluster_name))
        assert _relation_data(
            json_data=json_data,
            type_name="exposes",
            external_id_assert_fn=lambda eid:  pod_service_match.findall(eid)
        ).startswith("urn:/kubernetes:%s:service:%s:pod-service" % (cluster_name, namespace))
        # cluster-agent replicaset controls cluster-agent pod
        replicaset_controls_match = re.compile("urn:/kubernetes:%s:replicaset:stackstate-cluster-agent-.*->urn:/kubernetes:%s:pod:stackstate-cluster-agent-.*" % (cluster_name, cluster_name))
        assert _relation_data(
            json_data=json_data,
            type_name="controls",
            external_id_assert_fn=lambda eid:  replicaset_controls_match.findall(eid)
        ).startswith("urn:/kubernetes:%s:replicaset:stackstate-cluster-agent" % (cluster_name))
        # node-agent daemonset controls node-agent pod
        daemonset_controls_match = re.compile("urn:/kubernetes:%s:daemonset:stackstate-agent->urn:/kubernetes:%s:pod:stackstate-agent-.*" % (cluster_name, cluster_name))
        assert _relation_data(
            json_data=json_data,
            type_name="controls",
            external_id_assert_fn=lambda eid:  daemonset_controls_match.findall(eid)
        ).startswith("urn:/kubernetes:%s:daemonset:stackstate-agent" % (cluster_name))
    util.wait_until(wait_for_components, 120, 3)
