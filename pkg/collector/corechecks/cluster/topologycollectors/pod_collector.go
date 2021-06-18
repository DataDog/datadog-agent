// +build kubeapiserver

package topologycollectors

import (
	"fmt"

	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	v1 "k8s.io/api/core/v1"
)

// PodCollector implements the ClusterTopologyCollector interface.
type PodCollector struct {
	ComponentChan     chan<- *topology.Component
	RelationChan      chan<- *topology.Relation
	ContainerCorrChan chan<- *ContainerCorrelation
	VolumeCorrChan    chan<- *VolumeCorrelation
	ClusterTopologyCollector
}

// ContainerPort is used to keep state of the container ports.
type ContainerPort struct {
	HostPort      int32
	ContainerPort int32
}

// NewPodCollector
func NewPodCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation,
	containerCorrChannel chan<- *ContainerCorrelation, volumeCorrChannel chan<- *VolumeCorrelation,
	clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {

	return &PodCollector{
		ComponentChan:            componentChannel,
		RelationChan:             relationChannel,
		ContainerCorrChan:        containerCorrChannel,
		VolumeCorrChan:           volumeCorrChannel,
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

	// extract vars to reduce var creation count
	var component *topology.Component
	var controllerExternalID string
	for _, pod := range pods {
		// creates and publishes StackState pod component with relations
		component = pc.podToStackStateComponent(pod)
		pc.ComponentChan <- component

		// pod could not be scheduled for some reason
		if pod.Spec.NodeName != "" {
			pc.RelationChan <- pc.podToNodeStackStateRelation(pod)
		}

		managed := false
		// check to see if this pod is "managed" by a kubernetes controller
		for _, ref := range pod.OwnerReferences {
			switch kind := ref.Kind; kind {
			case DaemonSet:
				controllerExternalID = pc.buildDaemonSetExternalID(pod.Namespace, ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(controllerExternalID, component.ExternalID)
				managed = true
			case Deployment:
				controllerExternalID = pc.buildDeploymentExternalID(pod.Namespace, ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(controllerExternalID, component.ExternalID)
				managed = true
			case ReplicaSet:
				controllerExternalID = pc.buildReplicaSetExternalID(pod.Namespace, ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(controllerExternalID, component.ExternalID)
				managed = true
			case StatefulSet:
				controllerExternalID = pc.buildStatefulSetExternalID(pod.Namespace, ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(controllerExternalID, component.ExternalID)
				managed = true
			case Job:
				if pod.Status.Phase == "Succeeded" || pod.Status.Phase == "Failed" {
					// Pod finished running so we don't create the relation to its Job
					log.Debugf("skipping relation from pod: %s to finished job : %s", pod.Name, ref.Name)
					continue
				}
				controllerExternalID = pc.buildJobExternalID(pod.Namespace, ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(controllerExternalID, component.ExternalID)
				managed = true
			}
		}

		if !managed {
			pc.RelationChan <- pc.namespaceToPodStackStateRelation(pc.buildNamespaceExternalID(pod.Namespace), component.ExternalID)
		}

		for _, c := range pod.Spec.Containers {
			// map relations to config map
			for _, env := range c.EnvFrom {
				if env.ConfigMapRef != nil {
					pc.RelationChan <- pc.podToConfigMapStackStateRelation(component.ExternalID, pc.buildConfigMapExternalID(pod.Namespace, env.ConfigMapRef.LocalObjectReference.Name))
				} else if env.SecretRef != nil {
					pc.RelationChan <- pc.podToSecretStackStateRelation(component.ExternalID, pc.buildSecretExternalID(pod.Namespace, env.SecretRef.LocalObjectReference.Name))
				}
			}

			// map relations to config map for this variable
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
					pc.RelationChan <- pc.podToConfigMapVarStackStateRelation(component.ExternalID, pc.buildConfigMapExternalID(pod.Namespace, env.ValueFrom.ConfigMapKeyRef.LocalObjectReference.Name))
				} else if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					pc.RelationChan <- pc.podToSecretVarStackStateRelation(component.ExternalID, pc.buildSecretExternalID(pod.Namespace, env.ValueFrom.SecretKeyRef.LocalObjectReference.Name))
				}
			}
		}

		// Send the volume correlation
		if len(pod.Spec.Volumes) > 0 {
			volumeCorrelation := &VolumeCorrelation{
				Pod:        PodIdentifier{ExternalID: component.ExternalID, Namespace: pod.Namespace, Name: pod.Name, NodeName: pod.Spec.NodeName},
				Volumes:    pod.Spec.Volumes,
				Containers: pod.Spec.Containers,
			}

			log.Debugf("publishing volume correlation for Pod: %v", volumeCorrelation)
			pc.VolumeCorrChan <- volumeCorrelation
		}

		// send the containers to be correlated
		if len(pod.Status.ContainerStatuses) > 0 {
			containerCorrelation := &ContainerCorrelation{
				Pod:               ContainerPod{ExternalID: component.ExternalID, Name: pod.Name, Labels: pc.podTags(pod), PodIP: pod.Status.PodIP, Namespace: pod.Namespace, NodeName: pod.Spec.NodeName, Phase: string(pod.Status.Phase)},
				Containers:        pod.Spec.Containers,
				ContainerStatuses: pod.Status.ContainerStatuses,
			}
			log.Debugf("publishing container correlation for Pod: %v", containerCorrelation)
			pc.ContainerCorrChan <- containerCorrelation
		}
	}

	// close container correlation channel
	close(pc.ContainerCorrChan)
	// close container correlation channel
	close(pc.VolumeCorrChan)

	return nil
}

// Creates a StackState component from a Kubernetes / OpenShift Pod
func (pc *PodCollector) podToStackStateComponent(pod v1.Pod) *topology.Component {
	// creates a StackState component for the kubernetes pod
	log.Tracef("Mapping kubernetes pod to StackState Component: %s", pod.String())

	identifiers := make([]string, 0)

	if pod.Status.PodIP != "" {
		// We map the pod ip including clustername, namespace and podName because
		// the pod ip is not necessarily unique:
		// * Pods can use Host networking which gives them the ip of the host
		// * Pods for jobs can remain present after completion or failure (their status will not be running but Completed or Failed) 
		//   with their IP (that is now free again for reuse) still attached in the pod.Status
		identifiers = append(identifiers, fmt.Sprintf("urn:ip:/%s:%s:%s:%s", pc.GetInstance().URL, pod.Namespace, pod.Name, pod.Status.PodIP))
	}

	log.Tracef("Created identifiers for %s: %v", pod.Name, identifiers)

	podExternalID := pc.buildPodExternalID(pod.Namespace, pod.Name)

	// clear out the unnecessary status array values
	podStatus := pod.Status
	podStatus.Conditions = make([]v1.PodCondition, 0)
	podStatus.InitContainerStatuses = make([]v1.ContainerStatus, 0)
	podStatus.ContainerStatuses = make([]v1.ContainerStatus, 0)

	tags := pc.podTags(pod)

	component := &topology.Component{
		ExternalID: podExternalID,
		Type:       topology.Type{Name: "pod"},
		Data: map[string]interface{}{
			"name":              pod.Name,
			"creationTimestamp": pod.CreationTimestamp,
			"tags":              tags,
			"status":            podStatus,
			"identifiers":       identifiers,
			"uid":               pod.UID,
		},
	}

	component.Data.PutNonEmpty("generateName", pod.GenerateName)
	component.Data.PutNonEmpty("kind", pod.Kind)
	component.Data.PutNonEmpty("restartPolicy", pod.Spec.RestartPolicy)

	log.Tracef("Created StackState pod component %s: %v", podExternalID, component.JSONString())

	return component
}

// podTags creates the tags for a pod
func (pc *PodCollector) podTags(pod v1.Pod) map[string]string {
	tags := pc.initTags(pod.ObjectMeta)
	// add service account as a label to filter on
	if pod.Spec.ServiceAccountName != "" {
		tags["service-account"] = pod.Spec.ServiceAccountName
	}
	return tags
}

// Creates a StackState relation from a Kubernetes / OpenShift Pod to Node relation
func (pc *PodCollector) podToNodeStackStateRelation(pod v1.Pod) *topology.Relation {
	podExternalID := pc.buildPodExternalID(pod.Namespace, pod.Name)
	nodeExternalID := pc.buildNodeExternalID(pod.Spec.NodeName)

	log.Tracef("Mapping kubernetes pod to node relation: %s -> %s", podExternalID, nodeExternalID)

	relation := pc.CreateRelation(podExternalID, nodeExternalID, "scheduled_on")

	log.Tracef("Created StackState pod -> node relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState relation from a Kubernetes / OpenShift Controller Workload to Pod relation
func (pc *PodCollector) controllerWorkloadToPodStackStateRelation(controllerExternalID, podExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes controller workload to pod relation: %s -> %s", controllerExternalID, podExternalID)

	relation := pc.CreateRelation(controllerExternalID, podExternalID, "controls")

	log.Tracef("Created StackState controller workload -> pod relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState relation from a Kubernetes / OpenShift Pod to ConfigMap relation
func (pc *PodCollector) podToConfigMapStackStateRelation(podExternalID, configMapExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes pod to config map relation: %s -> %s", podExternalID, configMapExternalID)

	relation := pc.CreateRelation(podExternalID, configMapExternalID, "uses")

	log.Tracef("Created StackState pod -> config map relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState relation from a Kubernetes / OpenShift Pod to Secret relation
func (pc *PodCollector) podToSecretStackStateRelation(podExternalID, secretExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes pod to secret relation: %s -> %s", podExternalID, secretExternalID)

	relation := pc.CreateRelation(podExternalID, secretExternalID, "uses")

	log.Tracef("Created StackState pod -> secret relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState relation from a Kubernetes / OpenShift Pod to Namespace relation
func (pc *PodCollector) namespaceToPodStackStateRelation(namespaceExternalID, podExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes namespace to pod relation: %s -> %s", namespaceExternalID, podExternalID)

	relation := pc.CreateRelation(namespaceExternalID, podExternalID, "encloses")

	log.Tracef("Created StackState namespace -> pod relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState relation from a Kubernetes / OpenShift Pod to ConfigMap variable relation
func (pc *PodCollector) podToConfigMapVarStackStateRelation(podExternalID, configMapExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes pod to config map var relation: %s -> %s", podExternalID, configMapExternalID)

	relation := pc.CreateRelation(podExternalID, configMapExternalID, "uses_value")

	log.Tracef("Created StackState pod -> config map var relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState relation from a Kubernetes / OpenShift Pod to Secret variable relation
func (pc *PodCollector) podToSecretVarStackStateRelation(podExternalID, secretExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes pod to secret var relation: %s -> %s", podExternalID, secretExternalID)

	relation := pc.CreateRelation(podExternalID, secretExternalID, "uses_value")

	log.Tracef("Created StackState pod -> secret var relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}
