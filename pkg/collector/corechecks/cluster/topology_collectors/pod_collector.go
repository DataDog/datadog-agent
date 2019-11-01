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

// ContainerPort is used to keep state of the container ports.
type ContainerPort struct {
	HostPort int32
	ContainerPort int32
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
		pc.ComponentChan <- component
		pc.RelationChan <- pc.podToNodeStackStateRelation(pod)

		// check to see if this pod is "managed" by a kubernetes controller
		for _, ref := range pod.OwnerReferences {
			switch kind := ref.Kind; kind {
			case DaemonSet:
				dsExternalID := pc.buildDaemonSetExternalID(ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(dsExternalID, component.ExternalID)
			case Deployment:
				dmExternalID := pc.buildDeploymentExternalID(pod.Namespace, ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(dmExternalID, component.ExternalID)
			case ReplicaSet:
				rsExternalID := pc.buildReplicaSetExternalID(ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(rsExternalID, component.ExternalID)
			case StatefulSet:
				ssExternalID := pc.buildStatefulSetExternalID(ref.Name)
				pc.RelationChan <- pc.controllerWorkloadToPodStackStateRelation(ssExternalID, component.ExternalID)
			}
		}

		// map the volume components and relation to this pod
		for _, vol := range pod.Spec.Volumes {
			var volumeExternalID string
			if pc.isPersistentVolume(vol) {
				volumeExternalID = pc.buildPersistentVolumeExternalID(vol.Name)
			} else {
				volComponent := pc.volumeToStackStateComponent(pod, vol)
				volumeExternalID = volComponent.ExternalID
				pc.ComponentChan <- volComponent
			}

			pc.RelationChan <- pc.podToVolumeStackStateRelation(component.ExternalID, volumeExternalID)
		}

		// map container to exposed ports
		containerPorts := make(map[string]ContainerPort)
		for _, c := range pod.Spec.Containers {

			// map relations between the container and the volume
			for _, mount := range c.VolumeMounts {
				containerExternalID := pc.buildContainerExternalID(pod.Name, c.Name)
				volumeExternalID :=  pc.buildVolumeExternalID(mount.Name)
				pc.RelationChan <- pc.containerToVolumeStackStateRelation(containerExternalID, volumeExternalID, mount)
			}

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

			for _, port := range c.Ports {
				containerPorts[fmt.Sprintf("%s_%s", c.Image, c.Name)] = ContainerPort{
					HostPort: port.HostPort,
					ContainerPort: port.ContainerPort,
				}
			}
		}

		pc.ContainerCorrChan <- &ContainerCorrelation{pod.Spec.NodeName, pc.buildContainerMappingFunction(pod, component.ExternalID, containerPorts)}
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
		volume.Projected != nil || volume.PortworxVolume != nil || volume.ScaleIO != nil || volume.StorageOS != nil{
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
	if !pod.Spec.HostNetwork {
		// create identifier list to merge with StackState components
		identifiers = append(identifiers, fmt.Sprintf("urn:ip:/%s:%s", pc.GetInstance().URL, pod.Status.PodIP))
	} else {
		// map the pod ip (which is the host ip) with the pod name
		identifiers = append(identifiers, fmt.Sprintf("urn:ip:/%s:%s:%s", pc.GetInstance().URL, pod.Name, pod.Status.PodIP))
	}

	log.Tracef("Created identifiers for %s: %v", pod.Name, identifiers)

	podExternalID := pc.buildPodExternalID(pod.Name)

	// clear out the unnecessary status array values
	podStatus := pod.Status
	podStatus.Conditions = make([]v1.PodCondition, 0)
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
			"qosClass" : pod.Status.QOSClass,
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

	relation := pc.CreateRelation(podExternalID, nodeExternalID, "scheduled_on")

	log.Tracef("Created StackState pod -> node relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState component from a Kubernetes / OpenShift Pod Container
func (pc *PodCollector) containerToStackStateComponent(nodeIdentifier string, pod v1.Pod, container v1.ContainerStatus, containerPort ContainerPort) *topology.Component {
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
			"containerId": strippedContainerId,
		},
		"pod":          pod.Name,
		"podIP":          pod.Status.PodIP,
		"namespace":    pod.Namespace,
		"restartCount": container.RestartCount,
		"identifiers":  identifiers,
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

	relation := pc.CreateRelation(containerExternalID, podExternalID, "enclosed_in")

	log.Tracef("Created StackState container -> pod relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState relation from a Kubernetes / OpenShift Controller Workload to Pod relation
func (pc *PodCollector)  controllerWorkloadToPodStackStateRelation(controllerExternalID, podExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes controller workload to pod relation: %s -> %s", controllerExternalID, podExternalID)

	relation := pc.CreateRelation(controllerExternalID, podExternalID, "controls")

	log.Tracef("Created StackState controller workload -> pod relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState relation from a Kubernetes / OpenShift Pod to ConfigMap relation
func (pc *PodCollector)  podToConfigMapStackStateRelation(podExternalID, configMapExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes pod to config map relation: %s -> %s", podExternalID, configMapExternalID)

	relation := pc.CreateRelation(podExternalID, configMapExternalID, "uses")

	log.Tracef("Created StackState pod -> config map relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState relation from a Kubernetes / OpenShift Pod to ConfigMap variable relation
func (pc *PodCollector)  podToConfigMapVarStackStateRelation(podExternalID, configMapExternalID string) *topology.Relation {
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
			"name":   volume.Name,
			"source": volume.VolumeSource,
			"identifiers": identifiers,
			"tags":   tags,
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

// Create a StackState relation from a Kubernetes / OpenShift Container to a Volume
func (pc *PodCollector) containerToVolumeStackStateRelation(containerExternalID, volumeExternalID string, mount v1.VolumeMount) *topology.Relation {
	log.Tracef("Mapping kubernetes container to volume relation: %s -> %s", containerExternalID, volumeExternalID)

	data := map[string]interface{}{
		"name": mount.Name,
		"readOnly": mount.ReadOnly,
		"mountPath": mount.MountPath,
		"subPath" : mount.SubPath,
		"mountPropagation": mount.MountPropagation,
	}

	relation := pc.CreateRelationData(containerExternalID, volumeExternalID, "uses", data)

	log.Tracef("Created StackState container -> volume relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// build the function that is used to correlate container to nodes
func (pc *PodCollector)  buildContainerMappingFunction(pod v1.Pod, podExternalID string, containerPorts map[string]ContainerPort) func (nodeIdentifier string) (components []*topology.Component, relations []*topology.Relation) {
	return func (nodeIdentifier string) (components []*topology.Component, relations []*topology.Relation) {
		// creates a StackState component for the kubernetes pod containers + relation to pod
		for _, container := range pod.Status.ContainerStatuses {

			containerPort := ContainerPort{}
			if cntPort, found := containerPorts[fmt.Sprintf("%s_%s", container.Image, container.Name)]; found {
				containerPort = cntPort
			}

			// submit the StackState component for publishing to StackState
			containerComponent := pc.containerToStackStateComponent(nodeIdentifier, pod, container, containerPort)
			// create the relation between the container and pod
			containerRelation := pc.containerToPodStackStateRelation(containerComponent.ExternalID, podExternalID)

			components = append(components, containerComponent)
			relations = append(relations, containerRelation)
		}

		return components, relations
	}
}
