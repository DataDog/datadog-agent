// +build kubeapiserver

package topology_collectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/core/v1"
)

// PodCollector implements the ClusterTopologyCollector interface.
type PodCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan chan<- *topology.Relation
	ContainerCorrChan chan<- *ContainerCorrelation
	ClusterTopologyCollector
}

// NewPodCollector
func NewPodCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation,
	containerCorrChannel chan<- *ContainerCorrelation, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {

	return &PodCollector{
		ComponentChan: componentChannel,
		RelationChan: relationChannel,
		ContainerCorrChan: containerCorrChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *PodCollector) GetName() string {
	return "Pod Collector"
}

// Collects and Published the Pod Components
func (pc *PodCollector) CollectorFunction() error {
	pods, err := pc.GetAPIClient().GetPods()
	if err != nil {
		return err
	}

	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			return fmt.Errorf("could not find node for pod %s", pod.Name)
		}

		// creates and publishes StackState pod component with relations
		component := pc.podToStackStateComponent(pod)

		// check to see if this pod is "managed" by a kubernetes controller
		for _, ref := range pod.OwnerReferences {
			switch kind := ref.Kind; kind {
			case DaemonSet:
				dsExternalID := pc.buildDaemonSetExternalID(ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(dsExternalID, component.ExternalID)
			case Deployment:
				dmExternalID := pc.buildDeploymentExternalID(ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(dmExternalID, component.ExternalID)
			case ReplicaSet:
				rsExternalID := pc.buildReplicaSetExternalID(ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(rsExternalID, component.ExternalID)
			case StatefulSet:
				ssExternalID := pc.buildStatefulSetExternalID(ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(ssExternalID, component.ExternalID)
			}
		}

		pc.ComponentChan <- component
		pc.RelationChan <- pc.podToNodeStackStateRelation(pod)
		pc.ContainerCorrChan <- &ContainerCorrelation{pod.Spec.NodeName, pc.buildContainerMappingFunction(pod, component.ExternalID)}
	}

	// close container correlation channel
	close(pc.ContainerCorrChan)

	return nil
}

// Creates a StackState component from a Kubernetes / OpenShift Pod
func (pc *PodCollector) podToStackStateComponent(pod v1.Pod) *topology.Component {
	// creates a StackState component for the kubernetes pod
	log.Tracef("Mapping kubernetes pod to StackState Component: %s", pod.String())

	// create identifier list to merge with StackState components
	identifiers := []string{
		fmt.Sprintf("urn:ip:/%s:%s", pc.GetInstance().URL, pod.Status.PodIP),
	}
	log.Tracef("Created identifiers for %s: %v", pod.Name, identifiers)

	podExternalID := pc.buildPodExternalID(pod.Name)

	// clear out the unnecessary status array values
	podStatus := pod.Status
	podStatus.Conditions = make([]v1.PodCondition, 0)
	podStatus.ContainerStatuses = make([]v1.ContainerStatus, 0)

	tags := emptyIfNil(pod.Labels)
	tags = pc.addClusterNameTag(tags)

	component := &topology.Component{
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

// Creates a StackState relation from a Kubernetes / OpenShift Pod to Node relation
func (pc *PodCollector) podToNodeStackStateRelation(pod v1.Pod) *topology.Relation {
	podExternalID := pc.buildPodExternalID(pod.Name)
	nodeExternalID := pc.buildNodeExternalID(pod.Spec.NodeName)

	log.Tracef("Mapping kubernetes pod to node relation: %s -> %s", podExternalID, nodeExternalID)

	relation := &topology.Relation{
		ExternalID: fmt.Sprintf("%s->%s", podExternalID, nodeExternalID),
		SourceID:   podExternalID,
		TargetID:   nodeExternalID,
		Type:       topology.Type{Name: "scheduled_on"},
		Data:       map[string]interface{}{},
	}

	log.Tracef("Created StackState pod -> node relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState component from a Kubernetes / OpenShift Pod Container
func (pc *PodCollector) containerToStackStateComponent(nodeIdentifier string, pod v1.Pod, container v1.ContainerStatus) *topology.Component {
	log.Tracef("Mapping kubernetes pod container to StackState component: %s", container.String())
	// create identifier list to merge with StackState components

	identifier := ""
	strippedContainerId := extractLastFragment(container.ContainerID)
	if len(nodeIdentifier) > 0 {
		identifier = fmt.Sprintf("%s:%s", nodeIdentifier, strippedContainerId)
	} else {
		identifier = strippedContainerId
	}
	identifiers := []string{
		fmt.Sprintf("urn:container:/%s", identifier),
	}
	log.Tracef("Created identifiers for %s: %v", container.Name, identifiers)

	containerExternalID := pc.buildContainerExternalID(pod.Name, container.Name)

	tags := emptyIfNil(pod.Labels)
	tags = pc.addClusterNameTag(tags)

	data := map[string]interface{}{
		"name": container.Name,
		"docker": map[string]interface{}{
			"image":        container.Image,
			"container_id": strippedContainerId,
		},
		"pod":          pod.Name,
		"namespace":    pod.Namespace,
		"restartCount": container.RestartCount,
		"identifiers":  identifiers,
		"tags":         tags,
	}

	if container.State.Running != nil {
		data["startTime"] = container.State.Running.StartedAt
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
func (pc *PodCollector)  containerToPodStackStateRelation(containerExternalID, podExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes container to pod relation: %s -> %s", containerExternalID, podExternalID)

	relation := &topology.Relation{
		ExternalID: fmt.Sprintf("%s->%s", containerExternalID, podExternalID),
		SourceID:   containerExternalID,
		TargetID:   podExternalID,
		Type:       topology.Type{Name: "enclosed_in"},
		Data:       map[string]interface{}{},
	}

	log.Tracef("Created StackState container -> pod relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState relation from a Kubernetes / OpenShift Controller Workload to Pod relation
func (pc *PodCollector)  controllerWorkloadToPodStackStateRelation(controllerExternalID, podExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes controller workload to pod relation: %s -> %s", controllerExternalID, podExternalID)

	relation := &topology.Relation{
		ExternalID: fmt.Sprintf("%s->%s", controllerExternalID, podExternalID),
		SourceID:   controllerExternalID,
		TargetID:   podExternalID,
		Type:       topology.Type{Name: "controls"},
		Data:       map[string]interface{}{},
	}

	log.Tracef("Created StackState controller workload -> pod relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// build the function that is used to correlate container to nodes
func (pc *PodCollector)  buildContainerMappingFunction(pod v1.Pod, podExternalID string) func (nodeIdentifier string) (components []*topology.Component, relations []*topology.Relation) {
	return func (nodeIdentifier string) (components []*topology.Component, relations []*topology.Relation) {
		// creates a StackState component for the kubernetes pod containers + relation to pod
		for _, container := range pod.Status.ContainerStatuses {

			// submit the StackState component for publishing to StackState
			containerComponent := pc.containerToStackStateComponent(nodeIdentifier, pod, container)
			// create the relation between the container and pod
			containerRelation := pc.containerToPodStackStateRelation(containerComponent.ExternalID, podExternalID)

			components = append(components, containerComponent)
			relations = append(relations, containerRelation)
		}

		return components, relations
	}
}
