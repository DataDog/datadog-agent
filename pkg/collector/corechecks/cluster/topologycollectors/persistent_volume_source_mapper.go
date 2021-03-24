package topologycollectors

import (
	"fmt"
	"strings"

	"github.com/StackVista/stackstate-agent/pkg/topology"
	v1 "k8s.io/api/core/v1"
)

// PersistentVolumeSourceMapper maps a PersistentVolumeSource to an external Volume topology component
type PersistentVolumeSourceMapper func(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error)

var allPersistentVolumeSourceMappers = []PersistentVolumeSourceMapper{
	mapAwsEbsPersistentVolume,
	mapAzureDiskPersistentVolume,
	mapAzureFilePersistentVolume,
	mapCephFsPersistentVolume,
	mapCinderPersistentVolume,
	mapFCPersistentVolume,
	mapFlexPersistentVolume,
	mapFlockerPersistentVolume,
	mapGcePersistentDiskPersistentVolume,
	mapGlusterFsPersistentVolume,
	mapIscsiPersistentVolume,
	mapNfsPersistentVolume,
	mapPhotonPersistentDiskPersistentVolume,
	mapPortWorxPersistentVolume,
	mapQuobytePersistentVolume,
	mapRbdPersistentVolume,
	mapScaleIoPersistentVolume,
	mapStorageOsPersistentVolume,
	mapVspherePersistentVolume,
}

func mapAwsEbsPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.AWSElasticBlockStore == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("aws-ebs", volume.Spec.AWSElasticBlockStore.VolumeID, fmt.Sprint(volume.Spec.AWSElasticBlockStore.Partition))

	return pc.createStackStateVolumeComponent(volume, extID, nil)
}

func mapAzureDiskPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.AzureDisk == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("azure-disk", volume.Spec.AzureDisk.DiskName)

	return pc.createStackStateVolumeComponent(volume, extID, nil)
}

func mapAzureFilePersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.AzureFile == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("azure-file", volume.Spec.AzureFile.ShareName)

	return pc.createStackStateVolumeComponent(volume, extID, nil)
}

func mapCephFsPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.CephFS == nil {
		return nil, nil
	}

	components := func(idx int) []string {
		c := []string{volume.Spec.CephFS.Monitors[idx]}
		if volume.Spec.CephFS.Path != "" {
			c = append(c, volume.Spec.CephFS.Path)
		}
		return c
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("ceph-fs", components(0)...)

	idx := 1
	identifiers := []string{}

	for idx < len(volume.Spec.CephFS.Monitors) {
		identifiers = append(identifiers, pc.GetURNBuilder().BuildExternalVolumeExternalID("ceph-fs", components(idx)...))
		idx++
	}

	return pc.createStackStateVolumeComponent(volume, extID, identifiers)
}

func mapCinderPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.Cinder == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("cinder", volume.Spec.Cinder.VolumeID)

	return pc.createStackStateVolumeComponent(volume, extID, nil)
}

func mapFCPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.FC == nil {
		return nil, nil
	}

	ids := []string{}
	if len(volume.Spec.FC.TargetWWNs) > 0 {
		for _, wwn := range volume.Spec.FC.TargetWWNs {
			ids = append(ids, pc.GetURNBuilder().BuildExternalVolumeExternalID("fibre-channel", fmt.Sprintf("%s-lun-%d", wwn, *volume.Spec.FC.Lun)))
		}
	} else if len(volume.Spec.FC.WWIDs) > 0 {
		for _, wwid := range volume.Spec.FC.WWIDs {
			ids = append(ids, pc.GetURNBuilder().BuildExternalVolumeExternalID("fibre-channel", wwid))
		}
	} else {
		return nil, fmt.Errorf("Either volume.FC.TargetWWNs or volume.FC.WWIDs needs to be set")
	}

	extID := ids[0]
	identifiers := ids[1:]
	return pc.createStackStateVolumeComponent(volume, extID, identifiers)
}

func mapFlexPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.FlexVolume == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("flex", volume.Spec.FlexVolume.Driver)
	return pc.createStackStateVolumeComponent(volume, extID, nil)
}

// mapFlockerVolume DEPRECATED
func mapFlockerPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.Flocker == nil {
		return nil, nil
	}

	var extID string
	if volume.Spec.Flocker.DatasetName != "" {
		extID = pc.GetURNBuilder().BuildExternalVolumeExternalID("flocker", volume.Spec.Flocker.DatasetName)
	} else {
		extID = pc.GetURNBuilder().BuildExternalVolumeExternalID("flocker", volume.Spec.Flocker.DatasetUUID)
	}
	return pc.createStackStateVolumeComponent(volume, extID, nil)
}

func mapGcePersistentDiskPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.GCEPersistentDisk == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("gce-pd", volume.Spec.GCEPersistentDisk.PDName)
	return pc.createStackStateVolumeComponent(volume, extID, nil)
}

func mapGlusterFsPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.Glusterfs == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("gluster-fs", volume.Spec.Glusterfs.EndpointsName, volume.Spec.Glusterfs.Path)
	return pc.createStackStateVolumeComponent(volume, extID, nil)
}

func mapIscsiPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.ISCSI == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("iscsi", volume.Spec.ISCSI.TargetPortal, volume.Spec.ISCSI.IQN, fmt.Sprint(volume.Spec.ISCSI.Lun))

	identifiers := []string{}
	for _, tp := range volume.Spec.ISCSI.Portals {
		identifiers = append(identifiers, pc.GetURNBuilder().BuildExternalVolumeExternalID("iscsi", tp, volume.Spec.ISCSI.IQN, fmt.Sprint(volume.Spec.ISCSI.Lun)))
	}

	return pc.createStackStateVolumeComponent(volume, extID, identifiers)
}

func mapNfsPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.NFS == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("nfs", volume.Spec.NFS.Server, volume.Spec.NFS.Path)

	return pc.createStackStateVolumeComponent(volume, extID, nil)
}

func mapPhotonPersistentDiskPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.PhotonPersistentDisk == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("photon", volume.Spec.PhotonPersistentDisk.PdID)

	return pc.createStackStateVolumeComponent(volume, extID, nil)
}

func mapPortWorxPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.PortworxVolume == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("portworx", volume.Spec.PortworxVolume.VolumeID)

	return pc.createStackStateVolumeComponent(volume, extID, nil)
}

func mapQuobytePersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.Quobyte == nil {
		return nil, nil
	}

	ids := []string{}
	for _, reg := range strings.Split(volume.Spec.Quobyte.Registry, ",") {
		ids = append(ids, pc.GetURNBuilder().BuildExternalVolumeExternalID("quobyte", reg, volume.Spec.Quobyte.Volume))
	}

	extID := ids[0]
	return pc.createStackStateVolumeComponent(volume, extID, ids[1:])
}

func mapRbdPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.RBD == nil {
		return nil, nil
	}

	ids := []string{}
	for _, mon := range volume.Spec.RBD.CephMonitors {
		ids = append(ids, pc.GetURNBuilder().BuildExternalVolumeExternalID("rbd", mon, fmt.Sprintf("%s-image-%s", volume.Spec.RBD.RBDPool, volume.Spec.RBD.RBDImage)))
	}

	extID := ids[0]
	return pc.createStackStateVolumeComponent(volume, extID, ids[1:])
}

// mapScaleIoVolume DEPRECATED
func mapScaleIoPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.ScaleIO == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("scale-io", volume.Spec.ScaleIO.Gateway, volume.Spec.ScaleIO.System)

	return pc.createStackStateVolumeComponent(volume, extID, nil)
}

func mapStorageOsPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.StorageOS == nil {
		return nil, nil
	}

	ns := "default"
	if volume.Spec.StorageOS.VolumeNamespace != "" {
		ns = volume.Spec.StorageOS.VolumeNamespace
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("storage-os", ns, volume.Spec.StorageOS.VolumeName)
	return pc.createStackStateVolumeComponent(volume, extID, nil)
}

func mapVspherePersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.VsphereVolume == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("vsphere", volume.Spec.VsphereVolume.VolumePath)
	return pc.createStackStateVolumeComponent(volume, extID, nil)
}
