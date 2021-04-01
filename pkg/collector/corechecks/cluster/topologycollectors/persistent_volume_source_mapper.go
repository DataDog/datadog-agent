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

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("aws-ebs", strings.TrimPrefix(volume.Spec.AWSElasticBlockStore.VolumeID, "aws://"), fmt.Sprint(volume.Spec.AWSElasticBlockStore.Partition))

	tags := map[string]string{
		"kind":      "aws-ebs",
		"volume-id": volume.Spec.AWSElasticBlockStore.VolumeID,
		"partition": fmt.Sprint(volume.Spec.AWSElasticBlockStore.Partition),
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.AWSElasticBlockStore.VolumeID, extID, nil, tags)
}

func mapAzureDiskPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.AzureDisk == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("azure-disk", volume.Spec.AzureDisk.DiskName)

	tags := map[string]string{
		"kind":      "azure-disk",
		"disk-name": volume.Spec.AzureDisk.DiskName,
		"disk-uri":  volume.Spec.AzureDisk.DataDiskURI,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.AzureDisk.DiskName, extID, nil, tags)
}

func mapAzureFilePersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.AzureFile == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("azure-file", volume.Spec.AzureFile.ShareName)

	tags := map[string]string{
		"kind":       "azure-file",
		"share-name": volume.Spec.AzureFile.ShareName,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.AzureFile.ShareName, extID, nil, tags)
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

	tags := map[string]string{
		"kind": "ceph-fs",
		"path": volume.Spec.CephFS.Path,
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("ceph-fs", components(0)...)
	tags["monitors-0"] = volume.Spec.CephFS.Monitors[0]

	idx := 1
	identifiers := []string{}

	for idx < len(volume.Spec.CephFS.Monitors) {
		identifiers = append(identifiers, pc.GetURNBuilder().BuildExternalVolumeExternalID("ceph-fs", components(idx)...))
		tags[fmt.Sprintf("monitors-%d", idx)] = volume.Spec.CephFS.Monitors[idx]

		idx++
	}

	return pc.createStackStateVolumeSourceComponent(volume, "ceph-fs", extID, identifiers, tags)
}

func mapCinderPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.Cinder == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("cinder", volume.Spec.Cinder.VolumeID)

	tags := map[string]string{
		"kind":      "cinder",
		"volume-id": volume.Spec.Cinder.VolumeID,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.Cinder.VolumeID, extID, nil, tags)
}

func mapFCPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.FC == nil {
		return nil, nil
	}

	ids := []string{}

	tags := map[string]string{
		"kind": "fibre-channel",
	}

	if len(volume.Spec.FC.TargetWWNs) > 0 {
		for i, wwn := range volume.Spec.FC.TargetWWNs {
			ids = append(ids, pc.GetURNBuilder().BuildExternalVolumeExternalID("fibre-channel", fmt.Sprintf("%s-lun-%d", wwn, *volume.Spec.FC.Lun)))
			tags[fmt.Sprintf("wwn-%d", i)] = wwn
		}
		tags["lun"] = fmt.Sprint(*volume.Spec.FC.Lun)
	} else if len(volume.Spec.FC.WWIDs) > 0 {
		for i, wwid := range volume.Spec.FC.WWIDs {
			ids = append(ids, pc.GetURNBuilder().BuildExternalVolumeExternalID("fibre-channel", wwid))
			tags[fmt.Sprintf("wwid-%d", i)] = wwid
		}
	} else {
		return nil, fmt.Errorf("Either volume.FC.TargetWWNs or volume.FC.WWIDs needs to be set")
	}

	extID := ids[0]
	identifiers := ids[1:]

	return pc.createStackStateVolumeSourceComponent(volume, "fibre-channel", extID, identifiers, tags)
}

func mapFlexPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.FlexVolume == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("flex", volume.Spec.FlexVolume.Driver)

	tags := map[string]string{
		"kind":   "flex",
		"driver": volume.Spec.FlexVolume.Driver,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.FlexVolume.Driver, extID, nil, tags)
}

// mapFlockerVolume DEPRECATED
func mapFlockerPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.Flocker == nil {
		return nil, nil
	}

	tags := map[string]string{
		"kind": "flocker",
	}

	var extID string
	if volume.Spec.Flocker.DatasetName != "" {
		extID = pc.GetURNBuilder().BuildExternalVolumeExternalID("flocker", volume.Spec.Flocker.DatasetName)
		tags["dataset"] = volume.Spec.Flocker.DatasetName
	} else {
		extID = pc.GetURNBuilder().BuildExternalVolumeExternalID("flocker", volume.Spec.Flocker.DatasetUUID)
		tags["dataset"] = volume.Spec.Flocker.DatasetUUID
	}

	return pc.createStackStateVolumeSourceComponent(volume, tags["dataset"], extID, nil, tags)
}

func mapGcePersistentDiskPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.GCEPersistentDisk == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("gce-pd", volume.Spec.GCEPersistentDisk.PDName)

	tags := map[string]string{
		"kind":    "gce-pd",
		"pd-name": volume.Spec.GCEPersistentDisk.PDName,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.GCEPersistentDisk.PDName, extID, nil, tags)
}

func mapGlusterFsPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.Glusterfs == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("gluster-fs", volume.Spec.Glusterfs.EndpointsName, volume.Spec.Glusterfs.Path)

	tags := map[string]string{
		"kind":      "gluster-fs",
		"endpoints": volume.Spec.Glusterfs.EndpointsName,
		"path":      volume.Spec.Glusterfs.Path,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.Glusterfs.EndpointsName, extID, nil, tags)
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

	tags := map[string]string{
		"kind":          "iscsi",
		"target-portal": volume.Spec.ISCSI.TargetPortal,
		"iqn":           volume.Spec.ISCSI.IQN,
		"lun":           fmt.Sprint(volume.Spec.ISCSI.Lun),
		"interface":     volume.Spec.ISCSI.ISCSIInterface,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.ISCSI.TargetPortal, extID, identifiers, tags)
}

func mapNfsPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.NFS == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("nfs", volume.Spec.NFS.Server, volume.Spec.NFS.Path)

	tags := map[string]string{
		"kind":   "nfs",
		"server": volume.Spec.NFS.Server,
		"path":   volume.Spec.NFS.Path,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.NFS.Server, extID, nil, tags)
}

func mapPhotonPersistentDiskPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.PhotonPersistentDisk == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("photon", volume.Spec.PhotonPersistentDisk.PdID)

	tags := map[string]string{
		"kind":  "photon",
		"pd-id": volume.Spec.PhotonPersistentDisk.PdID,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.PhotonPersistentDisk.PdID, extID, nil, tags)
}

func mapPortWorxPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.PortworxVolume == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("portworx", volume.Spec.PortworxVolume.VolumeID)

	tags := map[string]string{
		"kind":      "portworx",
		"volume-id": volume.Spec.PortworxVolume.VolumeID,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.PortworxVolume.VolumeID, extID, nil, tags)
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

	tags := map[string]string{
		"kind":     "quobyte",
		"volume":   volume.Spec.Quobyte.Volume,
		"registry": volume.Spec.Quobyte.Registry,
		"user":     volume.Spec.Quobyte.User,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.Quobyte.Volume, extID, ids[1:], tags)
}

func mapRbdPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.RBD == nil {
		return nil, nil
	}

	ids := []string{}
	tags := map[string]string{
		"kind":  "rados",
		"pool":  volume.Spec.RBD.RBDPool,
		"image": volume.Spec.RBD.RBDImage,
	}

	for i, mon := range volume.Spec.RBD.CephMonitors {
		ids = append(ids, pc.GetURNBuilder().BuildExternalVolumeExternalID("rbd", mon, fmt.Sprintf("%s-image-%s", volume.Spec.RBD.RBDPool, volume.Spec.RBD.RBDImage)))
		tags[fmt.Sprintf("monitor-%d", i)] = mon
	}

	extID := ids[0]

	return pc.createStackStateVolumeSourceComponent(volume, "rados", extID, ids[1:], tags)
}

// mapScaleIoVolume DEPRECATED
func mapScaleIoPersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.ScaleIO == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("scale-io", volume.Spec.ScaleIO.Gateway, volume.Spec.ScaleIO.System)

	tags := map[string]string{
		"kind":              "scale-io",
		"gateway":           volume.Spec.ScaleIO.Gateway,
		"system":            volume.Spec.ScaleIO.System,
		"protection-domain": volume.Spec.ScaleIO.ProtectionDomain,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.ScaleIO.Gateway, extID, nil, tags)
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

	tags := map[string]string{
		"kind":             "storage-os",
		"volume":           volume.Spec.StorageOS.VolumeName,
		"volume-namespace": volume.Spec.StorageOS.VolumeNamespace,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.StorageOS.VolumeName, extID, nil, tags)
}

func mapVspherePersistentVolume(pc *PersistentVolumeCollector, volume v1.PersistentVolume) (*topology.Component, error) {
	if volume.Spec.VsphereVolume == nil {
		return nil, nil
	}

	extID := pc.GetURNBuilder().BuildExternalVolumeExternalID("vsphere", volume.Spec.VsphereVolume.VolumePath)

	tags := map[string]string{
		"kind":           "vsphere",
		"volume-path":    volume.Spec.VsphereVolume.VolumePath,
		"storage-policy": volume.Spec.VsphereVolume.StoragePolicyName,
	}

	return pc.createStackStateVolumeSourceComponent(volume, volume.Spec.VsphereVolume.VolumePath, extID, nil, tags)
}
