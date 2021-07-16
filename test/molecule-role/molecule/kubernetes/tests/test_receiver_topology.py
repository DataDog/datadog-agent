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


def _relation_sourceid(json_data, type_name, external_id_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyRelation" in p and \
            p["TopologyRelation"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyRelation"]["externalId"]):
            return p["TopologyRelation"]["sourceId"]
    return None


def _relation_data(json_data, type_name, external_id_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyRelation" in p and \
            p["TopologyRelation"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyRelation"]["externalId"]):
            return json.loads(p["TopologyRelation"]["data"])
    return None


def _find_relation(json_data, type_name, external_id_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyRelation" in p and \
            p["TopologyRelation"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyRelation"]["externalId"]):
            return p["TopologyRelation"]
    return None


def _find_component(json_data, type_name, external_id_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyComponent" in p and \
            p["TopologyComponent"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyComponent"]["externalId"]):
            return p["TopologyComponent"]
    return None


def _container_component(json_data, type_name, external_id_assert_fn, tags_assert_fn):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyComponent" in p and \
            p["TopologyComponent"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyComponent"]["externalId"]):
            component_data = json.loads(p["TopologyComponent"]["data"])
            if "tags" in component_data and tags_assert_fn(component_data["tags"]):
                return component_data
    return None


def _container_process_component(json_data, type_name, external_id_assert_fn, tags_assert_fn, containerId):
    for message in json_data["messages"]:
        p = message["message"]["TopologyElement"]["payload"]
        if "TopologyComponent" in p and \
            p["TopologyComponent"]["typeName"] == type_name and \
                external_id_assert_fn(p["TopologyComponent"]["externalId"]):
            component_data = json.loads(p["TopologyComponent"]["data"])
            if "tags" in component_data and tags_assert_fn(component_data["tags"]) and "containerId" in component_data and component_data["containerId"] == containerId:
                return component_data
    return None


def test_cluster_agent_base_topology(host, ansible_var):
    cluster_name = ansible_var("cluster_name")
    namespace = ansible_var("namespace")
    topic = "sts_topo_kubernetes_%s" % cluster_name
    url = "http://localhost:7070/api/topic/%s?limit=1000" % topic

    def wait_for_cluster_agent_components():
        data = host.check_output("curl \"%s\"" % url)
        json_data = json.loads(data)
        with open("./topic-" + topic + ".json", 'w') as f:
            json.dump(json_data, f, indent=4)

        process_data = host.check_output("curl \"%s\"" % "http://localhost:7070/api/topic/sts_topo_process_agents?limit=1000")
        process_json_data = json.loads(process_data)
        with open("./topic-topo-process-agents-k8s.json", 'w') as f:
            json.dump(process_json_data, f, indent=4)

        # 1 namespace
        assert _find_component(
            json_data=json_data,
            type_name="namespace",
            external_id_assert_fn=lambda eid: eid.startswith("urn:kubernetes:/%s:namespace/%s" % (cluster_name, namespace)),
        )
        # TODO make sure we identify the 2 different ec2 instances using i-*
        # 2 nodes
        assert _component_data(
            json_data=json_data,
            type_name="node",
            external_id_assert_fn=lambda eid: eid.startswith("urn:kubernetes:/%s:node/" % cluster_name),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:ip:/%s:" % cluster_name))
        )
        # 2 agent pods on each node, each pod 1 container
        assert _component_data(
            json_data=json_data,
            type_name="pod",
            external_id_assert_fn=lambda eid: eid.startswith("urn:kubernetes:/%s:%s:pod/stackstate-agent-" % (cluster_name, namespace)),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:ip:/%s:" % cluster_name))
        )
        node_agent_container_match = re.compile("urn:kubernetes:/%s:%s:pod/stackstate-agent-.*:"
                                                "container/stackstate-agent" % (cluster_name, namespace))
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
            external_id_assert_fn=lambda eid: eid.startswith("urn:kubernetes:/%s:%s:pod/"
                                                             "stackstate-cluster-agent-" % (cluster_name, namespace)),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:ip:/%s:" % cluster_name))
        )
        cluster_agent_container_match = re.compile("urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent-.*"
                                                   ":container/stackstate-cluster-agent" % (cluster_name, namespace))
        cluster_agent_container = _component_data(
            json_data=json_data,
            type_name="container",
            external_id_assert_fn=lambda eid: cluster_agent_container_match.findall(eid),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:container:/i-"))  # TODO ec2 i-*
        )
        stackstate_cluster_agent_container_external_id = cluster_agent_container["identifiers"][0]
        stackstate_cluster_agent_container_id = cluster_agent_container["docker"]["containerId"]
        stackstate_cluster_agent_container_pod = cluster_agent_container["pod"]
        # 2 service, one for each agent
        assert _component_data(
            json_data=json_data,
            type_name="service",
            external_id_assert_fn=lambda eid: eid.startswith("urn:kubernetes:/%s:%s:service/"
                                                             "stackstate-agent" % (cluster_name, namespace)),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:endpoint:/%s:" % cluster_name))
        )
        assert _component_data(
            json_data=json_data,
            type_name="service",
            external_id_assert_fn=lambda eid: eid.startswith("urn:kubernetes:/%s:%s:service/"
                                                             "stackstate-cluster-agent" % (cluster_name, namespace)),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:endpoint:/%s:" % cluster_name))
        )
        # 1 service, pod-service for dnat
        assert _component_data(
            json_data=json_data,
            type_name="service",
            external_id_assert_fn=lambda eid: eid.startswith("urn:kubernetes:/%s:%s:service/"
                                                             "pod-service" % (cluster_name, namespace)),
            cluster_name=cluster_name,
            identifiers_assert_fn=lambda identifiers: next(x for x in identifiers if x.startswith("urn:endpoint:/%s:" % cluster_name))
        )
        # 1 externalname service with associated external-service component
        assert _find_component(
            json_data=json_data,
            type_name="service",
            external_id_assert_fn=lambda eid: eid.startswith("urn:kubernetes:/%s:%s:service/"
                                                             "google-service" % (cluster_name, namespace)),
        )
        assert _find_component(
            json_data=json_data,
            type_name="external-service",
            external_id_assert_fn=lambda eid: eid.startswith("urn:kubernetes:/%s:%s:external-service/"
                                                             "google-service" % (cluster_name, namespace))
        )
        # 1 config map aws-auth
        configmap_match = re.compile("urn:kubernetes:/{}:{}:configmap/aws-auth"
                                     .format(cluster_name, "kube-system"))
        assert _find_component(
            json_data=json_data,
            type_name="configmap",
            external_id_assert_fn=lambda v: configmap_match.findall(v)
        )

        # 1 node agent config map sts-agent-config
        agent_configmap_match = re.compile("urn:kubernetes:/{}:{}:configmap/sts-agent-config"
                                           .format(cluster_name, namespace))
        assert _find_component(
            json_data=json_data,
            type_name="configmap",
            external_id_assert_fn=lambda v: agent_configmap_match.findall(v)
        )
        # 1 cluster agent config map sts-clusteragent-config
        cluster_agent_configmap_match = re.compile("urn:kubernetes:/{}:{}:configmap/"
                                                   "sts-clusteragent-config".format(cluster_name, namespace))
        assert _find_component(
            json_data=json_data,
            type_name="configmap",
            external_id_assert_fn=lambda v: cluster_agent_configmap_match.findall(v)
        )
        # 1 cluster agent secret stackstate-auth-token
        cluster_agent_secret_match = re.compile("urn:kubernetes:/{}:{}:secret/"
                                                "stackstate-auth-token".format(cluster_name, namespace))
        assert _find_component(
            json_data=json_data,
            type_name="secret",
            external_id_assert_fn=lambda v: cluster_agent_secret_match.findall(v)
        )
        # 1 node agent config map sts-agent-config
        agent_configmap_match = re.compile("urn:kubernetes:/{}:{}:configmap/"
                                           "sts-agent-config".format(cluster_name, namespace))
        assert _find_component(
            json_data=json_data,
            type_name="configmap",
            external_id_assert_fn=lambda v: agent_configmap_match.findall(v)
        )
        # 1 volume cgroups
        volume_match = re.compile("urn:kubernetes:external-volume:hostpath/.*/cgroup".format(cluster_name, namespace))
        assert _find_component(
            json_data=json_data,
            type_name="volume",
            external_id_assert_fn=lambda v: volume_match.findall(v)
        )

        # 1 replicaset cluster-agent
        replicaset_match = re.compile("urn:kubernetes:/{}:{}:replicaset/"
                                      "stackstate-cluster-agent-.*".format(cluster_name, namespace))
        assert _find_component(
            json_data=json_data,
            type_name="replicaset",
            external_id_assert_fn=lambda v: replicaset_match.findall(v)
        )
        # 1 deployment cluster-agent
        deployment_match = re.compile("urn:kubernetes:/{}:{}:deployment/"
                                      "stackstate-cluster-agent".format(cluster_name, namespace))
        assert _find_component(
            json_data=json_data,
            type_name="deployment",
            external_id_assert_fn=lambda v: deployment_match.findall(v)
        )
        # 1 daemonset node-agent
        daemonset_match = re.compile("urn:kubernetes:/{}:{}:daemonset/"
                                     "stackstate-agent".format(cluster_name, namespace))
        assert _find_component(
            json_data=json_data,
            type_name="daemonset",
            external_id_assert_fn=lambda v: daemonset_match.findall(v)
        )
        # 1 cronjob hello
        cronjob_match = re.compile("urn:kubernetes:/{}:{}:cronjob/hello".format(cluster_name, namespace))
        assert _find_component(
            json_data=json_data,
            type_name="cronjob",
            external_id_assert_fn=lambda v: cronjob_match.findall(v)
        )
        # 1 job countdown
        job_match = re.compile("urn:kubernetes:/{}:{}:job/countdown".format(cluster_name, namespace))
        assert _find_component(
            json_data=json_data,
            type_name="job",
            external_id_assert_fn=lambda v: job_match.findall(v)
        )
        # 1 persistent-volume
        persistent_volume_match = re.compile("urn:kubernetes:/{}:persistent-volume/pvc-.*"
                                             .format(cluster_name))
        assert _find_component(
            json_data=json_data,
            type_name="persistent-volume",
            external_id_assert_fn=lambda v: persistent_volume_match.findall(v)
        )
        # 1 statefulset mehdb
        statefulset_match = re.compile("urn:kubernetes:/{}:{}:statefulset/"
                                       "mehdb".format(cluster_name, namespace))
        assert _find_component(
            json_data=json_data,
            type_name="statefulset",
            external_id_assert_fn=lambda v: statefulset_match.findall(v)
        )
        # 1 ingress example-ingress
        ingress_match = re.compile("urn:kubernetes:/{}:{}:ingress/"
                                   "example-ingress".format(cluster_name, namespace))
        assert _find_component(
            json_data=json_data,
            type_name="ingress",
            external_id_assert_fn=lambda v: ingress_match.findall(v)
        )
        # Pod -> Node (scheduled on)
        # stackstate-agent pods is scheduled_on a node (2 times)
        node_agent_pod_scheduled_match = re.compile("urn:kubernetes:/%s:%s:pod/stackstate-agent-.*->"
                                                    "urn:kubernetes:/%s:node/ip-.*" % (cluster_name, namespace,
                                                                                       cluster_name))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="scheduled_on",
            external_id_assert_fn=lambda eid: node_agent_pod_scheduled_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:pod/stackstate-agent-" % (cluster_name, namespace))
        # stackstate-cluster-agent pod is scheduled_on a node (1 time)
        cluster_agent_pod_scheduled_match = re.compile("urn:kubernetes:/%s:%s:pod/"
                                                       "stackstate-cluster-agent-.*->urn:kubernetes:/%s:node/ip-.*" %
                                                       (cluster_name, namespace, cluster_name))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="scheduled_on",
            external_id_assert_fn=lambda eid: cluster_agent_pod_scheduled_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent" % (cluster_name, namespace))
        # Pod -> Container (encloses)
        # stackstate-agent pod encloses a container (2 times)
        node_agent_container_enclosed_match = re.compile(
            "urn:kubernetes:/%s:%s:pod/stackstate-agent-.*->"
            "urn:kubernetes:/%s:%s:pod/stackstate-agent-.*:container/stackstate-agent"
            % (cluster_name, namespace, cluster_name, namespace))
        pod_encloses_source_id = _relation_sourceid(
            json_data=json_data,
            type_name="encloses",
            external_id_assert_fn=lambda eid: node_agent_container_enclosed_match.findall(eid)
        )
        assert re.match(
            "urn:kubernetes:/%s:%s:pod/stackstate-agent-.*"
            % (cluster_name, namespace), pod_encloses_source_id)
        # stackstate-cluster-agent pod encloses a container (1 time)
        cluster_agent_container_enclosed_match = re.compile(
            "urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent-.*->"
            "urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent-.*:container/stackstate-cluster-agent"
            % (cluster_name, namespace, cluster_name, namespace))
        pod_encloses_source_id = _relation_sourceid(
            json_data=json_data,
            type_name="encloses",
            external_id_assert_fn=lambda eid: cluster_agent_container_enclosed_match.findall(eid)
        )
        assert re.match("urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent-.*" % (cluster_name, namespace), pod_encloses_source_id)
        # Pod -> Service (exposes)
        # stackstate-agent exposes stackstate-agent pods (2 times)
        node_agent_service_match = re.compile("urn:kubernetes:/%s:%s:service/stackstate-cluster-agent->"
                                              "urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent-.*" %
                                              (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="exposes",
            external_id_assert_fn=lambda eid:  node_agent_service_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:service/stackstate-cluster-agent" % (cluster_name, namespace))
        # stackstate-cluster-agent exposes stackstate-cluster-agent pod (1 time)
        cluster_agent_service_match = re.compile("urn:kubernetes:/%s:%s:service/stackstate-cluster-agent->"
                                                 "urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent-.*" %
                                                 (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="exposes",
            external_id_assert_fn=lambda eid:  cluster_agent_service_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:service/stackstate-cluster-agent" % (cluster_name, namespace))
        # pod-server  exposes pod-service(1 time)
        pod_service_match = re.compile("urn:kubernetes:/%s:%s:service/pod-service->"
                                       "urn:kubernetes:/%s:%s:pod/pod-server" %
                                       (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="exposes",
            external_id_assert_fn=lambda eid:  pod_service_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:service/pod-service" % (cluster_name, namespace))
        # cluster-agent replicaset controls cluster-agent pod
        replicaset_controls_match = re.compile("urn:kubernetes:/%s:%s:replicaset/stackstate-cluster-agent-.*"
                                               "->urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent-.*" %
                                               (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="controls",
            external_id_assert_fn=lambda eid:  replicaset_controls_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:replicaset/stackstate-cluster-agent" % (cluster_name, namespace))
        # node-agent daemonset controls node-agent pod
        daemonset_controls_match = re.compile("urn:kubernetes:/%s:%s:daemonset/stackstate-agent->"
                                              "urn:kubernetes:/%s:%s:pod/stackstate-agent-.*" %
                                              (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="controls",
            external_id_assert_fn=lambda eid:  daemonset_controls_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:daemonset/stackstate-agent" % (cluster_name, namespace))
        # cluster-agent deployment controls replicaset
        deployment_controls_match = re.compile("urn:kubernetes:/%s:%s:deployment/stackstate-cluster-agent->"
                                               "urn:kubernetes:/%s:%s:replicaset/stackstate-cluster-agent-.*"
                                               % (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="controls",
            external_id_assert_fn=lambda eid:  deployment_controls_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:deployment/stackstate-cluster-agent" % (cluster_name, namespace))
        #  statefulset controls pod
        statefulset_controls_match = re.compile("urn:kubernetes:/%s:%s:statefulset/mehdb->"
                                                "urn:kubernetes:/%s:%s:pod/mehdb-.*" %
                                                (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="controls",
            external_id_assert_fn=lambda eid:  statefulset_controls_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:statefulset/mehdb" % (cluster_name, namespace))
        #  cronjob creates job
        cronjob_creates_match = re.compile("urn:kubernetes:/%s:%s:cronjob/hello->"
                                           "urn:kubernetes:/%s:%s:job/hello-.*" %
                                           (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="creates",
            external_id_assert_fn=lambda eid:  cronjob_creates_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:cronjob/hello" % (cluster_name, namespace))
        #  pod claims volume
        pod_claims_volume_match = re.compile("urn:kubernetes:/%s:%s:pod/mehdb-1:container/shard->"
                                             "urn:kubernetes:/%s:persistent-volume/pvc-.*" %
                                             (cluster_name, namespace, cluster_name))
        assert _relation_data(
            json_data=json_data,
            type_name="mounts",
            external_id_assert_fn=lambda eid:  pod_claims_volume_match.findall(eid)
        )["mountPath"] == "/mehdbdata"
        #  pod claims HostPath volume
        pod_claims_persistent_volume_match = re.compile("urn:kubernetes:/%s:%s:pod/stackstate-agent-.*->"
                                                        "urn:kubernetes:external-volume:hostpath/.*/cgroup" %
                                                        (cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="mounts",
            external_id_assert_fn=lambda eid:  pod_claims_persistent_volume_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:pod/stackstate-agent" % (cluster_name, namespace))
        #  pod uses configmap cluster-agent -> sts-clusteragent-config
        pod_uses_configmap_match = re.compile("urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent-.*->"
                                              "urn:kubernetes:/%s:%s:configmap/sts-clusteragent-config" %
                                              (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="uses",
            external_id_assert_fn=lambda eid:  pod_uses_configmap_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent" % (cluster_name, namespace))
        #  pod uses_value secret cluster-agent -> stackstate-auth-token
        pod_uses_secret_match = re.compile("urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent-.*->"
                                           "urn:kubernetes:/%s:%s:secret/stackstate-auth-token" %
                                           (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="uses_value",
            external_id_assert_fn=lambda eid:  pod_uses_secret_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent" % (cluster_name, namespace))
        #  pod uses configmap node-agent -> sts-agent-config
        pod_uses_configmap_match = re.compile("urn:kubernetes:/%s:%s:pod/stackstate-agent-.*->"
                                              "urn:kubernetes:/%s:%s:configmap/sts-agent-config" %
                                              (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="uses",
            external_id_assert_fn=lambda eid:  pod_uses_configmap_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:pod/stackstate-agent" % (cluster_name, namespace))
        #  ingress routes service example-ingress -> bananna-service
        ingress_routes_service_match = re.compile("urn:kubernetes:/%s:%s:ingress/example-ingress->"
                                                  "urn:kubernetes:/%s:%s:service/banana-service" %
                                                  (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="routes",
            external_id_assert_fn=lambda eid:  ingress_routes_service_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:ingress/example-ingress" % (cluster_name, namespace))
        # stackstate-cluster-agent Container mounts Volume  stackstate-cluster-agent-token
        container_mounts_volume_match = re.compile(
            "urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent-.*:container/stackstate-cluster-agent->"
            "urn:kubernetes:/%s:%s:secret/stackstate-cluster-agent-token-.*"
            % (cluster_name, namespace, cluster_name, namespace)
        )
        assert _relation_sourceid(
            json_data=json_data,
            type_name="mounts",
            external_id_assert_fn=lambda eid:  container_mounts_volume_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:pod/stackstate-cluster-agent" % (cluster_name, namespace))
        # stackstate-cluster-agent Container mounts Volume  stackstate-cluster-agent-token
        agent_container_mounts_volume_match = \
            re.compile("urn:kubernetes:/%s:%s:pod/stackstate-agent-.*:container/stackstate-agent->"
                       "urn:kubernetes:/%s:%s:secret/stackstate-agent-token-.*" %
                       (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="mounts",
            external_id_assert_fn=lambda eid:  agent_container_mounts_volume_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:pod/stackstate-agent" % (cluster_name, namespace))
        # hello job controls hello pod
        job_controls_match = re.compile("urn:kubernetes:/%s:%s:job/countdown->"
                                        "urn:kubernetes:/%s:%s:pod/countdown-.*" %
                                        (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="controls",
            external_id_assert_fn=lambda eid:  job_controls_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:job/countdown" % (cluster_name, namespace))

        # assert process agent data
        stackstate_cluster_agent_process_match = re.compile("%s" % stackstate_cluster_agent_container_external_id)
        stackstate_cluster_agent_container = _container_component(
            json_data=process_json_data,
            type_name="container",
            external_id_assert_fn=lambda eid: stackstate_cluster_agent_process_match.findall(eid),
            tags_assert_fn=lambda tags: all([assertTag for assertTag in [
                "pod-name:%s" % stackstate_cluster_agent_container_pod,
                "namespace:%s" % namespace,
                "cluster-name:%s" % cluster_name
            ] if assertTag in tags])
        )

        stackstate_cluster_agent_process_match = re.compile("urn:process:/%s.*" %
                                                            stackstate_cluster_agent_container["host"])
        assert _container_process_component(
            json_data=process_json_data,
            type_name="process",
            external_id_assert_fn=lambda eid: stackstate_cluster_agent_process_match.findall(eid),
            tags_assert_fn=lambda tags: all([assertTag for assertTag in [
                "pod-name:%s" % stackstate_cluster_agent_container_pod,
                "namespace:%s" % namespace,
                "cluster-name:%s" % cluster_name
            ] if assertTag in tags]),
            containerId=stackstate_cluster_agent_container_id
        )
        # Assert Namespace relationships
        # Namespace -> Deployment (encloses)
        namespace_deployment_encloses_match = re.compile("urn:kubernetes:/%s:namespace/%s->"
                                                         "urn:kubernetes:/%s:%s:deployment/stackstate-cluster-agent" %
                                                         (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="encloses",
            external_id_assert_fn=lambda eid: namespace_deployment_encloses_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:namespace/%s" % (cluster_name, namespace))
        # Namespace -> StatefulSet (encloses)
        namespace_statefulset_encloses_match = re.compile("urn:kubernetes:/%s:namespace/%s->"
                                                          "urn:kubernetes:/%s:%s:statefulset/mehdb" %
                                                          (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="encloses",
            external_id_assert_fn=lambda eid: namespace_statefulset_encloses_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:namespace/%s" % (cluster_name, namespace))
        # Namespace -> DaemonSet (encloses)
        namespace_daemonset_encloses_match = re.compile("urn:kubernetes:/%s:namespace/%s->"
                                                        "urn:kubernetes:/%s:%s:daemonset/stackstate-agent" %
                                                        (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="encloses",
            external_id_assert_fn=lambda eid: namespace_daemonset_encloses_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:namespace/%s" % (cluster_name, namespace))
        # Namespace -> ReplicaSet does not exist as managed by Deployment
        namespace_daemonset_encloses_match = re.compile("urn:kubernetes:/%s:namespace/%s->"
                                                        "urn:kubernetes:/%s:%s:replicaset/stackstate-cluster-agent-.*" %
                                                        (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="encloses",
            external_id_assert_fn=lambda eid: namespace_daemonset_encloses_match.findall(eid)
        ) is None
        # Namespace -> Service (encloses)
        namespace_daemonset_encloses_match = re.compile("urn:kubernetes:/%s:namespace/%s->"
                                                        "urn:kubernetes:/%s:%s:service/stackstate-cluster-agent" %
                                                        (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="encloses",
            external_id_assert_fn=lambda eid: namespace_daemonset_encloses_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:namespace/%s" % (cluster_name, namespace))
        external_name_service_uses_external_match = re.compile("urn:kubernetes:/%s:%s:service/google-service->"
                                                               "urn:kubernetes:/%s:%s:external-service/google-service" %
                                                               (cluster_name, namespace, cluster_name, namespace))
        assert _relation_sourceid(
            json_data=json_data,
            type_name="uses",
            external_id_assert_fn=lambda eid: external_name_service_uses_external_match.findall(eid)
        ).startswith("urn:kubernetes:/%s:%s:service/google-service" % (cluster_name, namespace))

    util.wait_until(wait_for_cluster_agent_components, 120, 3)
