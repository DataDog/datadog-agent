// +build kubeapiserver

package topologycollectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/core/v1"
)

// PodCollector implements the ClusterTopologyCollector interface.
type PodCollector struct {
	ComponentChan     chan<- *topology.Component
	RelationChan      chan<- *topology.Relation
	ContainerCorrChan chan<- *ContainerCorrelation
	ClusterTopologyCollector
}

// ContainerPort is used to keep state of the container ports.
type ContainerPort struct {
	HostPort      int32
	ContainerPort int32
}

// NewPodCollector
func NewPodCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation,
	containerCorrChannel chan<- *ContainerCorrelation, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {

	return &PodCollector{
		ComponentChan:            componentChannel,
		RelationChan:             relationChannel,
		ContainerCorrChan:        containerCorrChannel,
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
	var volComponent *topology.Component
	var controllerExternalID string
	var volumeExternalID string
	for _, pod := range pods {
		// creates and publishes StackState pod component with relations
		component = pc.podToStackStateComponent(pod)
		pc.ComponentChan <- component
		// pod could not be scheduled for some reason
		if pod.Spec.NodeName != "" {
			pc.RelationChan <- pc.podToNodeStackStateRelation(pod)
		}

		// check to see if this pod is "managed" by a kubernetes controller
		for _, ref := range pod.OwnerReferences {
			switch kind := ref.Kind; kind {
			case DaemonSet:
				controllerExternalID = pc.buildDaemonSetExternalID(ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(controllerExternalID, component.ExternalID)
			case Deployment:
				controllerExternalID = pc.buildDeploymentExternalID(pod.Namespace, ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(controllerExternalID, component.ExternalID)
			case ReplicaSet:
				controllerExternalID = pc.buildReplicaSetExternalID(ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(controllerExternalID, component.ExternalID)
			case StatefulSet:
				controllerExternalID = pc.buildStatefulSetExternalID(ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(controllerExternalID, component.ExternalID)
			case Job:
				controllerExternalID = pc.buildJobExternalID(ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(controllerExternalID, component.ExternalID)
			}
		}

		// map the volume components and relation to this pod
		for _, vol := range pod.Spec.Volumes {
			if pc.isPersistentVolume(vol) {
				volumeExternalID = pc.buildPersistentVolumeExternalID(vol.Name)
			} else {
				volComponent = pc.volumeToStackStateComponent(pod, vol)
				volumeExternalID = volComponent.ExternalID
				pc.ComponentChan <- volComponent
			}

			pc.RelationChan <- pc.podToVolumeStackStateRelation(component.ExternalID, volumeExternalID)
		}

		for _, c := range pod.Spec.Containers {
			// map relations to config map
			for _, env := range c.EnvFrom {
				if env.ConfigMapRef != nil {
					pc.RelationChan <- pc.podToConfigMapStackStateRelation(component.ExternalID, pc.buildConfigMapExternalID(pod.Namespace, env.ConfigMapRef.LocalObjectReference.Name))
				}
			}

			// map relations to config map for this variable
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
					pc.RelationChan <- pc.podToConfigMapVarStackStateRelation(component.ExternalID, pc.buildConfigMapExternalID(pod.Namespace, env.ValueFrom.ConfigMapKeyRef.LocalObjectReference.Name))
				}
			}
		}

		// send the containers to be correlated
		if len(pod.Status.ContainerStatuses) > 0 {
			containerCorrelation := &ContainerCorrelation{
				Pod:               ContainerPod{ExternalID: component.ExternalID, Name: pod.Name, Labels: pod.Labels, PodIP: pod.Status.PodIP, Namespace: pod.Namespace, NodeName: pod.Spec.NodeName},
				Containers:        pod.Spec.Containers,
				ContainerStatuses: pod.Status.ContainerStatuses,
			}
			log.Debugf("publishing container correlation for Pod: %v", containerCorrelation)
			pc.ContainerCorrChan <- containerCorrelation
		}
	}

	// close container correlation channel
	close(pc.ContainerCorrChan)

	return nil
}

// Checks to see if the volume is a persistent volume
func (pc *PodCollector) isPersistentVolume(volume v1.Volume) bool {
	if volume.EmptyDir != nil || volume.Secret != nil || volume.ConfigMap != nil || volume.DownwardAPI != nil ||
		volume.Projected != nil {
		return false
	}

	// persistent volume types
	if volume.HostPath != nil || volume.GCEPersistentDisk != nil || volume.AWSElasticBlockStore != nil ||
		volume.NFS != nil || volume.ISCSI != nil || volume.Glusterfs != nil ||
		volume.RBD != nil || volume.FlexVolume != nil || volume.Cinder != nil || volume.CephFS != nil ||
		volume.Flocker != nil || volume.DownwardAPI != nil || volume.FC != nil || volume.AzureFile != nil ||
		volume.VsphereVolume != nil || volume.Quobyte != nil || volume.AzureDisk != nil || volume.PhotonPersistentDisk != nil ||
		volume.Projected != nil || volume.PortworxVolume != nil || volume.ScaleIO != nil || volume.StorageOS != nil {
		return true
	}

	return false
}

// Creates a StackState component from a Kubernetes / OpenShift Pod
func (pc *PodCollector) podToStackStateComponent(pod v1.Pod) *topology.Component {
	// creates a StackState component for the kubernetes pod
	log.Tracef("Mapping kubernetes pod to StackState Component: %s", pod.String())

	identifiers := make([]string, 0)
	// if the pod is using the host network do not map it as a identifier, this will cause all pods to merge.
	if pod.Status.PodIP != "" {
		if pod.Spec.HostNetwork {
			// create identifier list to merge with StackState components
			identifiers = append(identifiers, fmt.Sprintf("urn:ip:/%s:%s", pc.GetInstance().URL, pod.Status.PodIP))
		}

		// map the pod ip (which is the host ip) with the pod name
		identifiers = append(identifiers, fmt.Sprintf("urn:ip:/%s:%s:%s", pc.GetInstance().URL, pod.Name, pod.Status.PodIP))
	}

	log.Tracef("Created identifiers for %s: %v", pod.Name, identifiers)

	podExternalID := pc.buildPodExternalID(pod.Name)

	// clear out the unnecessary status array values
	podStatus := pod.Status
	podStatus.Conditions = make([]v1.PodCondition, 0)
	podStatus.InitContainerStatuses = make([]v1.ContainerStatus, 0)
	podStatus.ContainerStatuses = make([]v1.ContainerStatus, 0)

	tags := emptyIfNil(pod.Labels)
	// add service account as a label to filter on
	if pod.Spec.ServiceAccountName != "" {
		tags["service-account"] = pod.Spec.ServiceAccountName
	}
	tags = pc.addClusterNameTag(tags)

	component := &topology.Component{
		ExternalID: podExternalID,
		Type:       topology.Type{Name: "pod"},
		Data: map[string]interface{}{
			"name":              pod.Name,
			"creationTimestamp": pod.CreationTimestamp,
			"tags":              tags,
			"status":            podStatus,
			"namespace":         pod.Namespace,
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

// Creates a StackState relation from a Kubernetes / OpenShift Pod to Node relation
func (pc *PodCollector) podToNodeStackStateRelation(pod v1.Pod) *topology.Relation {
	podExternalID := pc.buildPodExternalID(pod.Name)
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

// Creates a StackState relation from a Kubernetes / OpenShift Pod to ConfigMap variable relation
func (pc *PodCollector) podToConfigMapVarStackStateRelation(podExternalID, configMapExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes pod to config map var relation: %s -> %s", podExternalID, configMapExternalID)

	relation := pc.CreateRelation(podExternalID, configMapExternalID, "uses_value")

	log.Tracef("Created StackState pod -> config map var relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState component from a Kubernetes / OpenShift Volume
func (pc *PodCollector) volumeToStackStateComponent(pod v1.Pod, volume v1.Volume) *topology.Component {
	// creates a StackState component for the kubernetes pod
	log.Tracef("Mapping kubernetes volume to StackState Component: %s", pod.String())

	volumeExternalID := pc.buildVolumeExternalID(volume.Name)

	identifiers := make([]string, 0)
	if volume.EmptyDir != nil {
		identifiers = append(identifiers, fmt.Sprintf("urn:/%s:%s:volume:%s:%s", pc.GetInstance().URL, pc.GetInstance().Type, pod.Spec.NodeName, volume.Name))
	}
	if volume.Secret != nil {
		identifiers = append(identifiers, fmt.Sprintf("urn/%s:%s:secret:%s", pc.GetInstance().URL, pc.GetInstance().Type, volume.Secret.SecretName))
	}
	if volume.DownwardAPI != nil {
		identifiers = append(identifiers, fmt.Sprintf("urn/%s:%s:downardapi:%s", pc.GetInstance().URL, pc.GetInstance().Type, volume.Name))
	}
	if volume.ConfigMap != nil {
		identifiers = append(identifiers, pc.buildConfigMapExternalID(pod.Namespace, volume.ConfigMap.Name))
	}
	if volume.Projected != nil {
		identifiers = append(identifiers, fmt.Sprintf("urn/%s:%s:projected:%s", pc.GetInstance().URL, pc.GetInstance().Type, volume.Name))
	}

	log.Tracef("Created identifiers for %s: %v", volume.Name, identifiers)

	tags := make(map[string]string, 0)
	tags = pc.addClusterNameTag(tags)

	component := &topology.Component{
		ExternalID: volumeExternalID,
		Type:       topology.Type{Name: "volume"},
		Data: map[string]interface{}{
			"name":        volume.Name,
			"source":      volume.VolumeSource,
			"identifiers": identifiers,
			"tags":        tags,
		},
	}

	log.Tracef("Created StackState volume component %s: %v", volumeExternalID, component.JSONString())

	return component
}

// Create a StackState relation from a Kubernetes / OpenShift Pod to a Volume
func (pc *PodCollector) podToVolumeStackStateRelation(podExternalID, volumeExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes pod to volume relation: %s -> %s", podExternalID, volumeExternalID)

	relation := pc.CreateRelation(podExternalID, volumeExternalID, "claims")

	log.Tracef("Created StackState pod -> volume relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}
