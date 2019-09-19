// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package kubeapi

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/autodiscovery/integration"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"gopkg.in/yaml.v2"
	"k8s.io/api/core/v1"
	"strings"
)

const (
	kubernetesAPITopologyCheckName = "kubernetes_api_topology"
)

type ClusterType string

const (
	Kubernetes ClusterType = "kubernetes"
	OpenShift              = "openshift"
)

// TopologyConfig is the config of the API server.
type TopologyConfig struct {
	ClusterName     string `yaml:"cluster_name"`
	CollectTopology bool   `yaml:"collect_topology"`
	CheckID         check.ID
	Instance        topology.Instance
}

// TopologyCheck grabs events from the API server.
type TopologyCheck struct {
	CommonCheck
	instance *TopologyConfig
}

type EndpointID struct {
	URL           string
	RefExternalID string
}

func (c *TopologyConfig) parse(data []byte) error {
	// default values
	c.ClusterName = config.Datadog.GetString("cluster_name")
	c.CollectTopology = config.Datadog.GetBool("collect_kubernetes_topology")

	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check.
func (t *TopologyCheck) Configure(config, initConfig integration.Data) error {
	err := t.ConfigureKubeApiCheck(config)
	if err != nil {
		return err
	}

	err = t.instance.parse(config)
	if err != nil {
		_ = log.Error("could not parse the config for the API topology check")
		return err
	}

	log.Debugf("Running config %s", config)
	return nil
}

// Run executes the check.
func (t *TopologyCheck) Run() error {
	// initialize kube api check
	err := t.InitKubeApiCheck()
	if err == apiserver.ErrNotLeader {
		log.Debug("Agent is not leader, will not run the check")
		return nil
	} else if err != nil {
		return err
	}

	// Running the event collection.
	if !t.instance.CollectTopology {
		return nil
	}

	// set the check "instance id" for snapshots
	t.instance.CheckID = kubernetesAPITopologyCheckName

	var instanceClusterType ClusterType
	switch openshiftPresence := t.ac.DetectOpenShiftAPILevel(); openshiftPresence {
	case apiserver.OpenShiftAPIGroup, apiserver.OpenShiftOAPI:
		instanceClusterType = OpenShift
	case apiserver.NotOpenShift:
		instanceClusterType = Kubernetes
	}
	t.instance.Instance = topology.Instance{Type: string(instanceClusterType), URL: t.instance.ClusterName}

	// start the topology snapshot with the batch-er
	batcher.GetBatcher().SubmitStartSnapshot(t.instance.CheckID, t.instance.Instance)

	// get all the nodes
	nodes, err := t.getAllNodes()
	if err != nil {
		return err
	}

	// get all the pods
	err = t.getAllPods(nodes)
	if err != nil {
		return err
	}

	// get all the services
	err = t.getAllServices()
	if err != nil {
		return err
	}

	// get all the containers
	batcher.GetBatcher().SubmitStopSnapshot(t.instance.CheckID, t.instance.Instance)
	batcher.GetBatcher().SubmitComplete(t.instance.CheckID)

	return nil
}

// get all the nodes in the k8s cluster
func (t *TopologyCheck) getAllNodes() ([]v1.Node, error) {
	nodes, err := t.ac.GetNodes()
	if err != nil {
		return nil, err
	}

	components := make([]*topology.Component, 0)

	for _, node := range nodes {
		// creates and publishes StackState node component
		component := t.mapAndSubmitNode(node)
		components = append(components, &component)
	}

	return nodes, nil
}

// get all the pods in the k8s cluster
func (t *TopologyCheck) getAllPods(nodes []v1.Node) error {
	pods, err := t.ac.GetPods()
	if err != nil {
		return err
	}

	components := make([]*topology.Component, 0)

	for _, pod := range pods {
		var node *v1.Node
		for _, n := range nodes {
			if n.Name == pod.Spec.NodeName {
				node = &n
				break
			}
		}
		if node == nil {
			return fmt.Errorf("Could not find node for pod %s", pod.Name)
		}

		// creates and publishes StackState pod component with relations
		component := t.mapAndSubmitPodWithRelations(*node, pod)
		components = append(components, &component)
	}

	return nil
}

// get all the services in the k8s cluster
func (t *TopologyCheck) getAllServices() error {
	services, err := t.ac.GetServices()
	if err != nil {
		return err
	}

	endpoints, err := t.ac.GetEndpoints()
	if err != nil {
		return err
	}

	components := make([]*topology.Component, 0)
	serviceEndpointIdentifiers := make(map[string][]EndpointID, 0)

	// Get all the endpoints for the Service
	for _, endpoint := range endpoints {
		serviceID := buildServiceID(endpoint.Namespace, endpoint.Name)
		for _, subset := range endpoint.Subsets {
			for _, address := range subset.Addresses {
				for _, port := range subset.Ports {
					endpointID := EndpointID{
						URL: fmt.Sprintf("%s:%d", address.IP, port.Port),
					}

					// check if the target reference is populated, so we can create relations
					if address.TargetRef != nil {
						switch kind := address.TargetRef.Kind; kind {
						// add endpoint url as identifier and create service -> pod relation
						case "Pod":
							endpointID.RefExternalID = t.buildPodExternalID(t.instance.ClusterName, address.TargetRef.Name)
						// ignore different Kind's for now, create no relation
						default:
						}
					}

					serviceEndpointIdentifiers[serviceID] = append(serviceEndpointIdentifiers[serviceID], endpointID)
				}
			}
		}
	}

	for _, service := range services {
		// creates and publishes StackState service component with relations
		serviceID := buildServiceID(service.Namespace, service.Name)
		component := t.mapAndSubmitService(service, serviceEndpointIdentifiers[serviceID])
		components = append(components, &component)
	}

	return nil
}

// Map and Submit the Kubernetes Node into a StackState component
func (t *TopologyCheck) mapAndSubmitNode(node v1.Node) topology.Component {

	// submit the StackState component for publishing to StackState
	component := t.nodeToStackStateComponent(node)
	log.Tracef("Publishing StackState node component for %s: %v", component.ExternalID, component.JSONString())
	batcher.GetBatcher().SubmitComponent(t.instance.CheckID, t.instance.Instance, component)

	return component
}

// Map and Submit the Kubernetes Pod into a StackState component
func (t *TopologyCheck) mapAndSubmitPodWithRelations(node v1.Node, pod v1.Pod) topology.Component {
	// submit the StackState component for publishing to StackState
	podComponent := t.podToStackStateComponent(pod)
	log.Tracef("Publishing StackState pod component for %s: %v", podComponent.ExternalID, podComponent.JSONString())
	batcher.GetBatcher().SubmitComponent(t.instance.CheckID, t.instance.Instance, podComponent)

	// creates a StackState relation for the kubernetes node -> pod
	relation := t.podToNodeStackStateRelation(pod)
	log.Tracef("Publishing StackState pod -> node relation %s->%s", relation.SourceID, relation.TargetID)
	batcher.GetBatcher().SubmitRelation(t.instance.CheckID, t.instance.Instance, relation)

	// creates a StackState component for the kubernetes pod containers + relation to pod
	for _, container := range pod.Status.ContainerStatuses {

		// submit the StackState component for publishing to StackState
		containerComponent := t.containerToStackStateComponent(node, pod, container)
		log.Tracef("Publishing StackState container component for %s: %v", containerComponent.ExternalID, containerComponent.JSONString())
		batcher.GetBatcher().SubmitComponent(t.instance.CheckID, t.instance.Instance, containerComponent)

		// create the relation between the container and pod
		relation := containerToPodStackStateRelation(containerComponent.ExternalID, podComponent.ExternalID)
		log.Tracef("Publishing StackState container -> pod relation %s->%s", relation.SourceID, relation.TargetID)
		batcher.GetBatcher().SubmitRelation(t.instance.CheckID, t.instance.Instance, relation)
	}

	return podComponent
}

// Map and Submit the Kubernetes Service into a StackState component with endpoints as identifiers
func (t *TopologyCheck) mapAndSubmitService(service v1.Service, endpoints []EndpointID) topology.Component {

	// submit the StackState component for publishing to StackState
	serviceComponent := t.serviceToStackStateComponent(service, endpoints)
	log.Tracef("Publishing StackState service component for %s: %v", serviceComponent.ExternalID, serviceComponent.JSONString())
	batcher.GetBatcher().SubmitComponent(t.instance.CheckID, t.instance.Instance, serviceComponent)

	for _, endpoint := range endpoints {
		// create the relation between the service and pod on the endpoint
		if endpoint.RefExternalID != "" {
			relation := podToServiceStackStateRelation(serviceComponent.ExternalID, endpoint.RefExternalID)
			log.Tracef("Publishing StackState service -> pod relation %s->%s", relation.SourceID, relation.TargetID)
			batcher.GetBatcher().SubmitRelation(t.instance.CheckID, t.instance.Instance, relation)
		}
	}

	return serviceComponent
}

// Creates a StackState component from a Kubernetes Node
func (t *TopologyCheck) nodeToStackStateComponent(node v1.Node) topology.Component {
	// creates a StackState component for the kubernetes node
	log.Tracef("Mapping kubernetes node to StackState component: %s", node.String())

	// create identifier list to merge with StackState components
	identifiers := make([]string, 0)
	for _, address := range node.Status.Addresses {
		switch addressType := address.Type; addressType {
		case v1.NodeInternalIP:
			identifiers = append(identifiers, fmt.Sprintf("urn:ip:/%s:%s:%s", t.instance.ClusterName, node.Name, address.Address))
		case v1.NodeExternalIP:
			identifiers = append(identifiers, fmt.Sprintf("urn:ip:/%s:%s", t.instance.ClusterName, address.Address))
		case v1.NodeHostName:
			//do nothing with it
		default:
			continue
		}
	}
	// this allow merging with host reported by main agent
	if len(node.Spec.ProviderID) > 0 {
		identifiers = append(identifiers, fmt.Sprintf("urn:host:/%s", extractInstanceIdFromProviderId(node.Spec)))
	}

	log.Tracef("Created identifiers for %s: %v", node.Name, identifiers)

	nodeExternalID := t.buildNodeExternalID(t.instance.ClusterName, node.Name)

	// clear out the unnecessary status array values
	nodeStatus := node.Status
	nodeStatus.Conditions = make([]v1.NodeCondition, 0)
	nodeStatus.Images = make([]v1.ContainerImage, 0)

	tags := emptyIfNil(node.Labels)
	tags = t.addClusterNameTag(tags)

	component := topology.Component{
		ExternalID: nodeExternalID,
		Type:       topology.Type{Name: "node"},
		Data: map[string]interface{}{
			"name":              node.Name,
			"kind":              node.Kind,
			"creationTimestamp": node.CreationTimestamp,
			"tags":              tags,
			"status":            nodeStatus,
			"namespace":         node.Namespace,
			"identifiers":       identifiers,
			//"taints": node.Spec.Taints,
		},
	}

	log.Tracef("Created StackState node component %s: %v", nodeExternalID, component.JSONString())

	return component
}

// Creates a StackState component from a Kubernetes Pod
func (t *TopologyCheck) podToStackStateComponent(pod v1.Pod) topology.Component {
	// creates a StackState component for the kubernetes pod
	log.Tracef("Mapping kubernetes pod to StackState Component: %s", pod.String())

	// create identifier list to merge with StackState components
	identifiers := []string{
		fmt.Sprintf("urn:ip:/%s:%s", t.instance.ClusterName, pod.Status.PodIP),
	}
	log.Tracef("Created identifiers for %s: %v", pod.Name, identifiers)

	podExternalID := t.buildPodExternalID(t.instance.ClusterName, pod.Name)

	// clear out the unnecessary status array values
	podStatus := pod.Status
	podStatus.Conditions = make([]v1.PodCondition, 0)
	podStatus.ContainerStatuses = make([]v1.ContainerStatus, 0)

	tags := emptyIfNil(pod.Labels)
	tags = t.addClusterNameTag(tags)

	component := topology.Component{
		ExternalID: podExternalID,
		Type:       topology.Type{Name: "pod"},
		Data: map[string]interface{}{
			"name":              pod.Name,
			"kind":              pod.Kind,
			"creationTimestamp": pod.CreationTimestamp,
			"tags":              tags,
			"status":            podStatus,
			"namespace":         pod.Namespace,
			//"tolerations": pod.Spec.Tolerations,
			"restartPolicy": pod.Spec.RestartPolicy,
			"identifiers":   identifiers,
			"uid":           pod.UID,
			"generateName":  pod.GenerateName,
		},
	}

	log.Tracef("Created StackState pod component %s: %v", podExternalID, component.JSONString())

	return component
}

// Creates a StackState relation from a Kubernetes Pod to Node relation
func (t *TopologyCheck) podToNodeStackStateRelation(pod v1.Pod) topology.Relation {
	podExternalID := t.buildPodExternalID(t.instance.ClusterName, pod.Name)
	nodeExternalID := t.buildNodeExternalID(t.instance.ClusterName, pod.Spec.NodeName)

	log.Tracef("Mapping kubernetes pod to node relation: %s -> %s", podExternalID, nodeExternalID)

	relation := topology.Relation{
		ExternalID: fmt.Sprintf("%s->%s", podExternalID, nodeExternalID),
		SourceID:   podExternalID,
		TargetID:   nodeExternalID,
		Type:       topology.Type{Name: "scheduled_on"},
		Data:       map[string]interface{}{},
	}

	log.Tracef("Created StackState pod -> node relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState component from a Kubernetes Pod Container
func (t *TopologyCheck) containerToStackStateComponent(node v1.Node, pod v1.Pod, container v1.ContainerStatus) topology.Component {
	log.Tracef("Mapping kubernetes pod container to StackState component: %s", container.String())
	// create identifier list to merge with StackState components

	identifier := ""
	strippedContainerId := extractLastFragment(container.ContainerID)
	if len(node.Spec.ProviderID) > 0 {
		identifier = fmt.Sprintf("%s:%s", extractInstanceIdFromProviderId(node.Spec), strippedContainerId)
	} else {
		identifier = strippedContainerId
	}
	identifiers := []string{
		fmt.Sprintf("urn:container:/%s", identifier),
	}
	log.Tracef("Created identifiers for %s: %v", container.Name, identifiers)

	containerExternalID := t.buildContainerExternalID(t.instance.ClusterName, pod.Name, container.Name)

	tags := emptyIfNil(pod.Labels)
	tags = t.addClusterNameTag(tags)

	data := map[string]interface{}{
		"name": container.Name,
		"docker": map[string]interface{}{
			"image":        container.Image,
			"container_id": strippedContainerId,
		},
		"restartCount": container.RestartCount,
		"identifiers":  identifiers,
		"tags":         tags,
	}

	if container.State.Running != nil {
		data["startTime"] = container.State.Running.StartedAt
	}

	component := topology.Component{
		ExternalID: containerExternalID,
		Type:       topology.Type{Name: "container"},
		Data:       data,
	}

	log.Tracef("Created StackState container component %s: %v", containerExternalID, component.JSONString())

	return component
}

// Creates a StackState relation from a Kubernetes Container to Pod relation
func containerToPodStackStateRelation(containerExternalID, podExternalID string) topology.Relation {
	log.Tracef("Mapping kubernetes container to pod relation: %s -> %s", containerExternalID, podExternalID)

	relation := topology.Relation{
		ExternalID: fmt.Sprintf("%s->%s", containerExternalID, podExternalID),
		SourceID:   containerExternalID,
		TargetID:   podExternalID,
		Type:       topology.Type{Name: "enclosed_in"},
		Data:       map[string]interface{}{},
	}

	log.Tracef("Created StackState container -> pod relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState component from a Kubernetes Pod Service
func (t *TopologyCheck) serviceToStackStateComponent(service v1.Service, endpoints []EndpointID) topology.Component {
	log.Tracef("Mapping kubernetes pod service to StackState component: %s", service.String())
	// create identifier list to merge with StackState components
	var identifiers []string
	serviceID := buildServiceID(service.Namespace, service.Name)

	// all external ip's which are associated with this service, but are not managed by kubernetes
	for _, ip := range service.Spec.ExternalIPs {
		for _, port := range service.Spec.Ports {
			identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%d", ip, port.Port))
		}
	}

	// all endpoints for this service
	for _, endpoint := range endpoints {
		identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%s", t.instance.ClusterName, endpoint.URL))
	}

	switch service.Spec.Type {
	case v1.ServiceTypeClusterIP:
		identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%s", t.instance.ClusterName, service.Spec.ClusterIP))
	case v1.ServiceTypeLoadBalancer:
		identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%s", t.instance.ClusterName, service.Spec.LoadBalancerIP))
	default:
	}

	log.Tracef("Created identifiers for %s: %v", service.Name, identifiers)

	serviceExternalID := t.buildServiceExternalID(t.instance.ClusterName, serviceID)

	tags := emptyIfNil(service.Labels)
	tags = t.addClusterNameTag(tags)

	data := map[string]interface{}{
		"name":              service.Name,
		"namespace":         service.Namespace,
		"creationTimestamp": service.CreationTimestamp,
		"tags":              tags,
		"identifiers":       identifiers,
	}

	if service.Status.LoadBalancer.Ingress != nil {
		data["ingressPoints"] = service.Status.LoadBalancer.Ingress
	}

	component := topology.Component{
		ExternalID: serviceExternalID,
		Type:       topology.Type{Name: "service"},
		Data:       data,
	}

	log.Tracef("Created StackState service component %s: %v", serviceExternalID, component.JSONString())

	return component
}

// Creates a StackState component from a Kubernetes Pod Service
func podToServiceStackStateRelation(refExternalID, serviceExternalID string) topology.Relation {
	log.Tracef("Mapping kubernetes reference to service relation: %s -> %s", refExternalID, serviceExternalID)

	relation := topology.Relation{
		ExternalID: fmt.Sprintf("%s->%s", refExternalID, serviceExternalID),
		SourceID:   refExternalID,
		TargetID:   serviceExternalID,
		Type:       topology.Type{Name: "exposes"},
		Data:       map[string]interface{}{},
	}

	log.Tracef("Created StackState reference -> service relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

func (t *TopologyCheck) addClusterNameTag(tags map[string]string) map[string]string {
	tags["cluster-name"] = t.instance.ClusterName
	return tags
}

func (t *TopologyCheck) buildNodeExternalID(clusterName, nodeName string) string {
	return fmt.Sprintf("urn:/%s:%s:node:%s", t.instance.Instance.Type, clusterName, nodeName)
}

func (t *TopologyCheck) buildPodExternalID(clusterName, podName string) string {
	return fmt.Sprintf("urn:/%s:%s:pod:%s", t.instance.Instance.Type, clusterName, podName)
}

func (t *TopologyCheck) buildContainerExternalID(clusterName, podName, containerName string) string {
	return fmt.Sprintf("urn:/%s:%s:pod:%s:container:%s", t.instance.Instance.Type, clusterName, podName, containerName)
}

func (t *TopologyCheck) buildServiceExternalID(clusterName, serviceID string) string {
	return fmt.Sprintf("urn:/%s:%s:service:%s", t.instance.Instance.Type, clusterName, serviceID)
}

func emptyIfNil(m map[string]string) map[string]string {
	if m == nil {
		m = make(map[string]string, 0)
	}
	return m
}

func extractLastFragment(value string) string {
	lastSlash := strings.LastIndex(value, "/")
	return value[lastSlash+1:]
}

func extractInstanceIdFromProviderId(spec v1.NodeSpec) string {
	//parse node id from cloud provider (for AWS is the ec2 instance id)
	return extractLastFragment(spec.ProviderID)
}

// buildServiceID - combination of the service namespace and service name
func buildServiceID(serviceNamespace, serviceName string) string {
	return fmt.Sprintf("%s:%s", serviceNamespace, serviceName)
}

// KubernetesASFactory is exported for integration testing.
func KubernetesApiTopologyFactory() check.Check {
	return &TopologyCheck{
		CommonCheck: CommonCheck{
			CheckBase: core.NewCheckBase(kubernetesAPITopologyCheckName),
		},
		instance: &TopologyConfig{},
	}
}

func init() {
	core.RegisterCheck(kubernetesAPITopologyCheckName, KubernetesApiTopologyFactory)
}
