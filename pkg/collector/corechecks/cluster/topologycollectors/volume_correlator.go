// +build kubeapiserver

package topologycollectors

import (
	"fmt"

	"github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/urn"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ ClusterTopologyCorrelator = (*VolumeCorrelator)(nil)

type VolumeCreator interface {
	CreateStackStateVolumeSourceComponent(pod PodIdentifier, volume v1.Volume, externalID string, identifiers []string, addTags map[string]string) (*VolumeComponentsToCreate, error)
	GetURNBuilder() urn.Builder
	CreateRelation(sourceExternalID, targetExternalID, typeName string) *topology.Relation
}

// PodIdentifier resembles the identifying information of the Pod which needs to be correlated
type PodIdentifier struct {
	ExternalID string
	Namespace  string
	Name       string
	NodeName   string
}

// VolumeCorrelation is the transfer object which is used to correlate a Pod and its Containers with the Volumes they use
type VolumeCorrelation struct {
	Pod        PodIdentifier
	Volumes    []v1.Volume
	Containers []v1.Container
}

// VolumeCorrelator is the correlation function which relates Pods and their Containers with the Volumes in use.
type VolumeCorrelator struct {
	ComponentChan  chan<- *topology.Component
	RelationChan   chan<- *topology.Relation
	VolumeCorrChan <-chan *VolumeCorrelation
	ClusterTopologyCorrelator
}

// NewVolumeCorrelator instantiates the VolumeCorrelator
func NewVolumeCorrelator(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation, volumeCorrChannel chan *VolumeCorrelation, clusterTopologyCorrelator ClusterTopologyCorrelator) ClusterTopologyCorrelator {
	return &VolumeCorrelator{
		ComponentChan:             componentChannel,
		RelationChan:              relationChannel,
		VolumeCorrChan:            volumeCorrChannel,
		ClusterTopologyCorrelator: clusterTopologyCorrelator,
	}
}

// GetName returns the name of the Correlator
func (_ *VolumeCorrelator) GetName() string {
	return "Volume Correlator"
}

// CorrelateFunction executes the Pod/Container to Volume correlation
func (vc *VolumeCorrelator) CorrelateFunction() error {
	pvcLookup, err := vc.buildPersistentVolumeClaimLookup()
	if err != nil {
		return err
	}

	for volumeCorrelation := range vc.VolumeCorrChan {
		pod := volumeCorrelation.Pod
		volumeLookup := map[string]string{}

		for _, volume := range volumeCorrelation.Volumes {
			volumeExternalID, err := vc.mapVolumeAndRelationToStackState(pod, volume, pvcLookup)
			if err != nil {
				return err
			}

			if volumeExternalID != "" {
				volumeLookup[volume.Name] = volumeExternalID
			}
		}

		for _, container := range volumeCorrelation.Containers {
			for _, mount := range container.VolumeMounts {
				volumeExternalID, ok := volumeLookup[mount.Name]
				if !ok {
					log.Errorf("Container '%s' of Pod '%s' mounts an unknown volume '%s'", container.Name, pod.ExternalID, mount.Name)

					continue
				}

				containerExternalID := vc.buildContainerExternalID(pod.Namespace, pod.Name, container.Name)

				vc.RelationChan <- vc.containerToVolumeStackStateRelation(containerExternalID, volumeExternalID, mount)
			}
		}
	}
	return nil
}

// buildPersistentVolumeClaimLookup builds a lookup table of PersistentVolumeClaim.Name to PersistentVolume.Name
func (vc *VolumeCorrelator) buildPersistentVolumeClaimLookup() (map[string]string, error) {
	pvcMapping := map[string]string{}

	pvcs, err := vc.GetAPIClient().GetPersistentVolumeClaims()
	if err != nil {
		return nil, err
	}

	for _, persistentVolumeClaim := range pvcs {
		pvcMapping[persistentVolumeClaim.Name] = vc.buildPersistentVolumeExternalID(persistentVolumeClaim.Spec.VolumeName)
	}

	return pvcMapping, nil
}

// mapVolumeAndRelationToStackState sends (potential) Volume component to StackState and relates it to the Pod, returning the ExternalID of the Volume component
func (vc *VolumeCorrelator) mapVolumeAndRelationToStackState(pod PodIdentifier, volume v1.Volume, pvcMapping map[string]string) (string, error) {
	var volumeExternalID string

	if volume.DownwardAPI != nil {
		return "", nil // The downward API does not need a volume
	} else if volume.PersistentVolumeClaim != nil {
		claimedPVExtID, ok := pvcMapping[volume.PersistentVolumeClaim.ClaimName]

		if !ok {
			log.Errorf("Unknown PersistentVolumeClaim '%s' referenced from Pod '%s'", volume.PersistentVolumeClaim.ClaimName, pod.ExternalID)

			return "", nil
		}

		volumeExternalID = claimedPVExtID
	} else {
		var toCreate *VolumeComponentsToCreate
		var err error
		for _, mapper := range allVolumeSourceMappers {
			toCreate, err = mapper(vc, pod, volume)
			if err != nil {
				return "", err
			}

			if toCreate != nil {
				break
			}
		}

		// From v1.Volume:
		// VolumeSource represents the location and type of the mounted volume.
		// If not specified, the Volume is implied to be an EmptyDir.
		// This implied behavior is deprecated and will be removed in a future version.
		if toCreate == nil {
			volumeExternalID = vc.GetURNBuilder().BuildVolumeExternalID("empty-dir", fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, volume.Name))

			tags := map[string]string{
				"kind": "empty-dir",
			}

			toCreate, err = vc.CreateStackStateVolumeSourceComponent(pod, volume, volumeExternalID, nil, tags)
			if err != nil {
				return "", err
			}
		}

		for _, c := range toCreate.Components {
			vc.ComponentChan <- c
		}

		for _, r := range toCreate.Relations {
			vc.RelationChan <- r
		}

		volumeExternalID = toCreate.VolumeExternalID
	}

	vc.RelationChan <- vc.podToVolumeStackStateRelation(pod.ExternalID, volumeExternalID)
	return volumeExternalID, nil
}

// Create a StackState relation from a Kubernetes / OpenShift Pod to a Volume
func (vc *VolumeCorrelator) podToVolumeStackStateRelation(podExternalID, volumeExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes pod to volume relation: %s -> %s", podExternalID, volumeExternalID)

	relation := vc.CreateRelation(podExternalID, volumeExternalID, "claims")

	log.Tracef("Created StackState pod -> volume relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Create a StackState relation from a Kubernetes / OpenShift Container to a Volume
func (vc *VolumeCorrelator) containerToVolumeStackStateRelation(containerExternalID, volumeExternalID string, mount v1.VolumeMount) *topology.Relation {
	log.Tracef("Mapping kubernetes container to volume relation: %s -> %s", containerExternalID, volumeExternalID)

	data := map[string]interface{}{
		"name":             mount.Name,
		"readOnly":         mount.ReadOnly,
		"mountPath":        mount.MountPath,
		"subPath":          mount.SubPath,
		"mountPropagation": mount.MountPropagation,
	}

	relation := vc.CreateRelationData(containerExternalID, volumeExternalID, "mounts", data)

	log.Tracef("Created StackState container -> volume relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Create a StackState relation from a Kubernetes / OpenShift Projected Volume to a Projection
func (vc *VolumeCorrelator) projectedVolumeToProjectionStackStateRelation(projectedVolumeExternalID, projectionExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes projected volume to projection relation: %s -> %s", projectedVolumeExternalID, projectionExternalID)

	relation := vc.CreateRelation(projectedVolumeExternalID, projectionExternalID, "projects")

	log.Tracef("Created StackState projected volume -> projection relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

func (vc *VolumeCorrelator) CreateStackStateVolumeSourceComponent(pod PodIdentifier, volume v1.Volume, externalID string, identifiers []string, addTags map[string]string) (*VolumeComponentsToCreate, error) {

	tags := vc.initTags(metav1.ObjectMeta{Namespace: pod.Namespace})
	for k, v := range addTags {
		tags[k] = v
	}

	data := map[string]interface{}{
		"name":   volume.Name,
		"source": volume.VolumeSource,
		"tags":   tags,
	}

	if identifiers != nil {
		data["identifiers"] = identifiers
	}

	component := &topology.Component{
		ExternalID: externalID,
		Type:       topology.Type{Name: "volume"},
		Data:       data,
	}

	log.Tracef("Created StackState volume component %s: %v", externalID, component.JSONString())

	return &VolumeComponentsToCreate{
		Components:       []*topology.Component{component},
		Relations:        []*topology.Relation{},
		VolumeExternalID: component.ExternalID,
	}, nil
}
