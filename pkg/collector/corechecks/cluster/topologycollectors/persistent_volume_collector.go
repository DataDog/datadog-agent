// +build kubeapiserver

package topologycollectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	v1 "k8s.io/api/core/v1"
)

// PersistentVolumeCollector implements the ClusterTopologyCollector interface.
type PersistentVolumeCollector struct {
	ComponentChan chan<- *topology.Component
	ClusterTopologyCollector
}

// NewPersistentVolumeCollector
func NewPersistentVolumeCollector(componentChannel chan<- *topology.Component, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &PersistentVolumeCollector{
		ComponentChan:            componentChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *PersistentVolumeCollector) GetName() string {
	return "Persistent Volume Collector"
}

// Collects and Published the Persistent Volume Components
func (pvc *PersistentVolumeCollector) CollectorFunction() error {
	persistentVolumes, err := pvc.GetAPIClient().GetPersistentVolumes()
	if err != nil {
		return err
	}

	for _, pv := range persistentVolumes {
		pvc.ComponentChan <- pvc.persistentVolumeToStackStateComponent(pv)
	}

	return nil
}

// Creates a Persistent Volume StackState component from a Kubernetes / OpenShift Cluster
func (pvc *PersistentVolumeCollector) persistentVolumeToStackStateComponent(persistentVolume v1.PersistentVolume) *topology.Component {
	log.Tracef("Mapping PersistentVolume to StackState component: %s", persistentVolume.String())

	identifiers := make([]string, 0)
	//dataSource := make(map[string]interface{}, 0)
	//if persistentVolume.Spec.HostPath != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s", pvc.GetInstance().URL, persistentVolume.Spec.HostPath.Path))
	//}
	//if persistentVolume.Spec.GCEPersistentDisk != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s", pvc.GetInstance().URL, persistentVolume.Spec.GCEPersistentDisk.PDName))
	//}
	//if persistentVolume.Spec.AWSElasticBlockStore != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s", pvc.GetInstance().URL, persistentVolume.Spec.AWSElasticBlockStore.VolumeID))
	//}
	//if persistentVolume.Spec.NFS != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s", pvc.GetInstance().URL, persistentVolume.Spec.NFS.Server))
	//}
	//if persistentVolume.Spec.ISCSI != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s", pvc.GetInstance().URL, persistentVolume.Spec.ISCSI.IQN))
	//}
	//if persistentVolume.Spec.Glusterfs != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s", pvc.GetInstance().URL, persistentVolume.Spec.Glusterfs.Path))
	//}
	//if persistentVolume.Spec.RBD != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s:%s", pvc.GetInstance().URL, persistentVolume.Spec.RBD.RadosUser, persistentVolume.Spec.RBD.RBDPool))
	//}
	//if persistentVolume.Spec.FlexVolume != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s:%s", pvc.GetInstance().URL, persistentVolume.Spec.FlexVolume.Driver, persistentVolume.Spec.FlexVolume.SecretRef.Name))
	//}
	//if persistentVolume.Spec.Cinder != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s", pvc.GetInstance().URL, persistentVolume.Spec.Cinder.VolumeID ))
	//}
	//if persistentVolume.Spec.CephFS != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:cepfs:/%s:%s:%s", pvc.GetInstance().URL, persistentVolume.Spec.CephFS.User, persistentVolume.Spec.CephFS.Path))
	//}
	//if persistentVolume.Spec.Flocker != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s:%s", pvc.GetInstance().URL, pod.Name, ))
	//}
	//if persistentVolume.Spec.FC != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s:%s", pvc.GetInstance().URL, pod.Name, ))
	//}
	//if persistentVolume.Spec.AzureFile != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s:%s", pvc.GetInstance().URL, pod.Name, ))
	//}
	//if persistentVolume.Spec.VsphereVolume != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s:%s", pvc.GetInstance().URL, pod.Name, ))
	//}
	//if persistentVolume.Spec.Quobyte != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s:%s", pvc.GetInstance().URL, pod.Name, ))
	//}
	//if persistentVolume.Spec.AzureDisk != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s:%s", pvc.GetInstance().URL, pod.Name, ))
	//}
	//if persistentVolume.Spec.PhotonPersistentDisk != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s:%s", pvc.GetInstance().URL, pod.Name, ))
	//}
	//if persistentVolume.Spec.PortworxVolume != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s:%s", pvc.GetInstance().URL, pod.Name, ))
	//}
	//if persistentVolume.Spec.ScaleIO != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s:%s", pvc.GetInstance().URL, pod.Name, ))
	//}
	//if persistentVolume.Spec.StorageOS != nil {
	//	identifiers = append(identifiers, fmt.Sprintf("urn:persistent-volume:/%s:%s:%s", pvc.GetInstance().URL, pod.Name, ))
	//}

	log.Tracef("Created identifiers for %s: %v", persistentVolume.Name, identifiers)

	persistentVolumeExternalID := pvc.buildPersistentVolumeExternalID(persistentVolume.Name)

	tags := emptyIfNil(persistentVolume.Labels)
	// add service account as a label to filter on
	tags = pvc.addClusterNameTag(tags)

	component := &topology.Component{
		ExternalID: persistentVolumeExternalID,
		Type:       topology.Type{Name: "persistent-volume"},
		Data: map[string]interface{}{
			"name":              persistentVolume.Name,
			"creationTimestamp": persistentVolume.CreationTimestamp,
			"tags":              tags,
			"namespace":         persistentVolume.Namespace,
			"uid":               persistentVolume.UID,
			"identifiers":       identifiers,
			"status":            persistentVolume.Status.Phase,
			"statusMessage":     persistentVolume.Status.Message,
			"storageClassName":  persistentVolume.Spec.StorageClassName,
			"source":            persistentVolume.Spec.PersistentVolumeSource,
		},
	}

	component.Data.PutNonEmpty("kind", persistentVolume.Kind)
	component.Data.PutNonEmpty("generateName", persistentVolume.GenerateName)

	log.Tracef("Created StackState persistent volume component %s: %v", persistentVolumeExternalID, component.JSONString())

	return component
}
