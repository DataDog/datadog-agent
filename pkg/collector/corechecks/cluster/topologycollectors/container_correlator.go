// +build kubeapiserver

package topologycollectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/core/v1"
)

// ContainerToNodeCorrelation
type NodeIdentifierCorrelation struct {
	NodeName       string
	NodeIdentifier string
}

// ContainerPod
type ContainerPod struct {
	ExternalID string
	Name       string
	Labels     map[string]string
	PodIP      string
	Namespace  string
	NodeName   string
}

// ContainerCorrelation
type ContainerCorrelation struct {
	Pod               ContainerPod
	Containers        []v1.Container
	ContainerStatuses []v1.ContainerStatus
}

// ContainerCorrelator implements the ClusterTopologyCollector interface.
type ContainerCorrelator struct {
	ComponentChan          chan<- *topology.Component
	RelationChan           chan<- *topology.Relation
	NodeIdentifierCorrChan <-chan *NodeIdentifierCorrelation
	ContainerCorrChan      <-chan *ContainerCorrelation
	ClusterTopologyCorrelator
}

// NewContainerCorrelator
func NewContainerCorrelator(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation,
	nodeIdentifierCorrChan <-chan *NodeIdentifierCorrelation, containerCorrChannel <-chan *ContainerCorrelation, clusterTopologyCorrelator ClusterTopologyCorrelator) ClusterTopologyCorrelator {
	return &ContainerCorrelator{
		ComponentChan:             componentChannel,
		RelationChan:              relationChannel,
		NodeIdentifierCorrChan:    nodeIdentifierCorrChan,
		ContainerCorrChan:         containerCorrChannel,
		ClusterTopologyCorrelator: clusterTopologyCorrelator,
	}
}

// GetName returns the name of the Collector
func (_ *ContainerCorrelator) GetName() string {
	return "Container Correlator"
}

// Collects and Published the Cluster Component
func (cc *ContainerCorrelator) CorrelateFunction() error {

	nodeMap := make(map[string]string)
	// map containers that require the Node instanceId
	for containerToNodeCorrelation := range cc.NodeIdentifierCorrChan {
		nodeMap[containerToNodeCorrelation.NodeName] = containerToNodeCorrelation.NodeIdentifier
	}

	for containerCorrelation := range cc.ContainerCorrChan {
		pod := containerCorrelation.Pod
		// map container to exposed ports
		containerPorts := make(map[string]ContainerPort)
		for _, c := range containerCorrelation.Containers {

			// map relations between the container and the volume
			for _, mount := range c.VolumeMounts {
				containerExternalID := cc.buildContainerExternalID(pod.Name, c.Name)
				volumeExternalID := cc.buildVolumeExternalID(mount.Name)
				cc.RelationChan <- cc.containerToVolumeStackStateRelation(containerExternalID, volumeExternalID, mount)
			}

			for _, port := range c.Ports {
				containerPorts[fmt.Sprintf("%s_%s", c.Image, c.Name)] = ContainerPort{
					HostPort:      port.HostPort,
					ContainerPort: port.ContainerPort,
				}
			}
		}

		// check to see if we have container statuses
		for _, container := range containerCorrelation.ContainerStatuses {
			containerPort := ContainerPort{}
			if cntPort, found := containerPorts[fmt.Sprintf("%s_%s", container.Image, container.Name)]; found {
				containerPort = cntPort
			}

			if nodeIdentifier, ok := nodeMap[pod.NodeName]; ok {
				// submit the StackState component for publishing to StackState
				containerComponent := cc.containerToStackStateComponent(nodeIdentifier, pod, container, containerPort)
				cc.ComponentChan <- containerComponent
				// create the relation between the container and pod
				cc.RelationChan <- cc.containerToPodStackStateRelation(containerComponent.ExternalID, pod.ExternalID)
			}
		}
	}

	return nil
}

// Creates a StackState component from a Kubernetes / OpenShift Pod Container
func (cc *ContainerCorrelator) containerToStackStateComponent(nodeIdentifier string, pod ContainerPod, container v1.ContainerStatus, containerPort ContainerPort) *topology.Component {
	log.Tracef("Mapping kubernetes pod container to StackState component: %s", container.String())
	// create identifier list to merge with StackState components

	var identifiers []string
	strippedContainerId := extractLastFragment(container.ContainerID)
	// in the case where the container could not be started due to some error
	if len(strippedContainerId) > 0 {
		identifier := ""
		if len(nodeIdentifier) > 0 {
			identifier = fmt.Sprintf("%s:%s", nodeIdentifier, strippedContainerId)
		} else {
			identifier = strippedContainerId
		}
		identifiers = []string{
			fmt.Sprintf("urn:container:/%s", identifier),
		}
	}

	log.Tracef("Created identifiers for %s: %v", container.Name, identifiers)

	containerExternalID := cc.buildContainerExternalID(pod.Name, container.Name)

	tags := emptyIfNil(pod.Labels)
	tags = cc.addClusterNameTag(tags)

	data := map[string]interface{}{
		"name": container.Name,
		"docker": map[string]interface{}{
			"image":       container.Image,
			"containerId": strippedContainerId,
		},
		"pod":          pod.Name,
		"podIP":        pod.PodIP,
		"namespace":    pod.Namespace,
		"restartCount": container.RestartCount,
		"tags":         tags,
	}

	if container.State.Running != nil {
		data["startTime"] = container.State.Running.StartedAt
	}

	if containerPort.ContainerPort != 0 {
		data["containerPort"] = containerPort.ContainerPort
	}

	if containerPort.HostPort != 0 {
		data["hostPort"] = containerPort.HostPort
	}

	if len(identifiers) > 0 {
		data["identifiers"] = identifiers
	}

	component := &topology.Component{
		ExternalID: containerExternalID,
		Type:       topology.Type{Name: "container"},
		Data:       data,
	}

	log.Tracef("Created StackState container component %s: %v", containerExternalID, component.JSONString())

	return component
}

// Creates a StackState relation from a Kubernetes / OpenShift Container to Pod relation
func (cc *ContainerCorrelator) containerToPodStackStateRelation(containerExternalID, podExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes container to pod relation: %s -> %s", containerExternalID, podExternalID)

	relation := cc.CreateRelation(containerExternalID, podExternalID, "enclosed_in")

	log.Tracef("Created StackState container -> pod relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Create a StackState relation from a Kubernetes / OpenShift Container to a Volume
func (cc *ContainerCorrelator) containerToVolumeStackStateRelation(containerExternalID, volumeExternalID string, mount v1.VolumeMount) *topology.Relation {
	log.Tracef("Mapping kubernetes container to volume relation: %s -> %s", containerExternalID, volumeExternalID)

	data := map[string]interface{}{
		"name":             mount.Name,
		"readOnly":         mount.ReadOnly,
		"mountPath":        mount.MountPath,
		"subPath":          mount.SubPath,
		"mountPropagation": mount.MountPropagation,
	}

	relation := cc.CreateRelationData(containerExternalID, volumeExternalID, "mounts", data)

	log.Tracef("Created StackState container -> volume relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}
