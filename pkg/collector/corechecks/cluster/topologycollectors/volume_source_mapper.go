// +build kubeapiserver

package topologycollectors

import (
	"fmt"
	"strings"

	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"github.com/pborman/uuid"
	v1 "k8s.io/api/core/v1"
)

// VolumeComponentsToCreate is the return type for the VolumeSourceMapper, indicating all StackState topology components and relations that need to be published and the externalID of the volume component
type VolumeComponentsToCreate struct {
	Components       []*topology.Component
	Relations        []*topology.Relation
	VolumeExternalID string
}

// VolumeSourceMapper maps a VolumeSource to an external Volume topology component externalID
type VolumeSourceMapper func(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error)

var allVolumeSourceMappers = []VolumeSourceMapper{
	createAwsEbsVolume,
	createAzureDiskVolume,
	createAzureFileVolume,
	createCephFsVolume,
	createCinderVolume,
	createConfigMapVolume,
	createEmptyDirVolume,
	createFCVolume,
	createFlexVolume,
	createFlockerVolume,
	createGcePersistentDiskVolume,
	createGitRepoVolume,
	createGlusterFsVolume,
	createHostPathVolume,
	createIscsiVolume,
	createNfsVolume,
	createPhotonPersistentDiskVolume,
	createPortWorxVolume,
	createProjectedVolume,
	createQuobyteVolume,
	createRbdVolume,
	createScaleIoVolume,
	createSecretVolume,
	createStorageOsVolume,
	createVsphereVolume,
}

func createAwsEbsVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.AWSElasticBlockStore == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("aws-ebs", strings.TrimPrefix(volume.AWSElasticBlockStore.VolumeID, "aws://"), fmt.Sprint(volume.AWSElasticBlockStore.Partition))

	tags := map[string]string{
		"kind":      "aws-ebs",
		"volume-id": volume.AWSElasticBlockStore.VolumeID,
		"partition": fmt.Sprint(volume.AWSElasticBlockStore.Partition),
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createAzureDiskVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.AzureDisk == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("azure-disk", volume.AzureDisk.DiskName)

	tags := map[string]string{
		"kind":      "azure-disk",
		"disk-name": volume.AzureDisk.DiskName,
		"disk-uri":  volume.AzureDisk.DataDiskURI,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createAzureFileVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.AzureFile == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("azure-file", volume.AzureFile.ShareName)

	tags := map[string]string{
		"kind":       "azure-file",
		"share-name": volume.AzureFile.ShareName,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createCephFsVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.CephFS == nil {
		return nil, nil
	}

	tags := map[string]string{
		"kind": "ceph-fs",
		"path": volume.CephFS.Path,
	}

	components := func(idx int) []string {
		c := []string{volume.CephFS.Monitors[idx]}
		if volume.CephFS.Path != "" {
			c = append(c, volume.CephFS.Path)
		}
		return c
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("ceph-fs", components(0)...)
	tags["monitors-0"] = volume.CephFS.Monitors[0]

	idx := 1
	identifiers := []string{}

	for idx < len(volume.CephFS.Monitors) {
		identifiers = append(identifiers, vc.GetURNBuilder().BuildExternalVolumeExternalID("ceph-fs", components(idx)...))
		tags[fmt.Sprintf("monitors-%d", idx)] = volume.CephFS.Monitors[idx]

		idx++
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, identifiers, tags)
}

func createCinderVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.Cinder == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("cinder", volume.Cinder.VolumeID)

	tags := map[string]string{
		"kind":      "cinder",
		"volume-id": volume.Cinder.VolumeID,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createConfigMapVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.ConfigMap == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildConfigMapExternalID(pod.Namespace, volume.ConfigMap.Name)

	return &VolumeComponentsToCreate{
		Components:       []*topology.Component{},
		Relations:        []*topology.Relation{},
		VolumeExternalID: extID,
	}, nil
}

func createEmptyDirVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.EmptyDir == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildVolumeExternalID("empty-dir", fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, volume.Name))

	tags := map[string]string{
		"kind": "empty-dir",
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createFCVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.FC == nil {
		return nil, nil
	}

	ids := []string{}

	tags := map[string]string{
		"kind": "fibre-channel",
	}

	if len(volume.FC.TargetWWNs) > 0 {
		for i, wwn := range volume.FC.TargetWWNs {
			ids = append(ids, vc.GetURNBuilder().BuildExternalVolumeExternalID("fibre-channel", fmt.Sprintf("%s-lun-%d", wwn, *volume.FC.Lun)))
			tags[fmt.Sprintf("wwn-%d", i)] = wwn
		}
		tags["lun"] = fmt.Sprint(*volume.FC.Lun)

	} else if len(volume.FC.WWIDs) > 0 {
		for i, wwid := range volume.FC.WWIDs {
			ids = append(ids, vc.GetURNBuilder().BuildExternalVolumeExternalID("fibre-channel", wwid))
			tags[fmt.Sprintf("wwid-%d", i)] = wwid

		}
	} else {
		return nil, fmt.Errorf("Either volume.FC.TargetWWNs or volume.FC.WWIDs needs to be set")
	}

	extID := ids[0]
	identifiers := ids[1:]
	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, identifiers, tags)
}

func createFlexVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.FlexVolume == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("flex", volume.FlexVolume.Driver)

	tags := map[string]string{
		"kind":   "flex",
		"driver": volume.FlexVolume.Driver,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

// createFlockerVolume DEPRECATED
func createFlockerVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.Flocker == nil {
		return nil, nil
	}

	tags := map[string]string{
		"kind": "flocker",
	}

	var extID string
	if volume.Flocker.DatasetName != "" {
		extID = vc.GetURNBuilder().BuildExternalVolumeExternalID("flocker", volume.Flocker.DatasetName)
		tags["dataset"] = volume.Flocker.DatasetName
	} else {
		extID = vc.GetURNBuilder().BuildExternalVolumeExternalID("flocker", volume.Flocker.DatasetUUID)
		tags["dataset"] = volume.Flocker.DatasetUUID
	}
	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createGcePersistentDiskVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.GCEPersistentDisk == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("gce-pd", volume.GCEPersistentDisk.PDName)

	tags := map[string]string{
		"kind":    "gce-pd",
		"pd-name": volume.GCEPersistentDisk.PDName,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

// createGitRepoVolume DEPRECATED
func createGitRepoVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.GitRepo == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("git-repo", volume.GitRepo.Repository)

	tags := map[string]string{
		"kind":       "git-repo",
		"repository": volume.GitRepo.Repository,
		"revision":   volume.GitRepo.Revision,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createGlusterFsVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.Glusterfs == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("gluster-fs", volume.Glusterfs.EndpointsName, volume.Glusterfs.Path)

	tags := map[string]string{
		"kind":      "gluster-fs",
		"endpoints": volume.Glusterfs.EndpointsName,
		"path":      volume.Glusterfs.Path,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createHostPathVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.HostPath == nil {
		return nil, nil
	} else if pod.NodeName == "" { // Not scheduled yet...
		return nil, nil
	}

	// The hostpath starts with a '/', strip that as it leads to a double '/' in the externalID
	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("hostpath", pod.NodeName, strings.TrimPrefix(volume.HostPath.Path, "/"))

	tags := map[string]string{
		"kind":     "hostpath",
		"nodename": pod.NodeName,
		"path":     volume.HostPath.Path,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createIscsiVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.ISCSI == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("iscsi", volume.ISCSI.TargetPortal, volume.ISCSI.IQN, fmt.Sprint(volume.ISCSI.Lun))

	identifiers := []string{}
	for _, tp := range volume.ISCSI.Portals {
		identifiers = append(identifiers, vc.GetURNBuilder().BuildExternalVolumeExternalID("iscsi", tp, volume.ISCSI.IQN, fmt.Sprint(volume.ISCSI.Lun)))
	}

	tags := map[string]string{
		"kind":          "iscsi",
		"target-portal": volume.ISCSI.TargetPortal,
		"iqn":           volume.ISCSI.IQN,
		"lun":           fmt.Sprint(volume.ISCSI.Lun),
		"interface":     volume.ISCSI.ISCSIInterface,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, identifiers, tags)
}

func createNfsVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.NFS == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("nfs", volume.NFS.Server, volume.NFS.Path)

	tags := map[string]string{
		"kind":   "nfs",
		"server": volume.NFS.Server,
		"path":   volume.NFS.Path,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createPhotonPersistentDiskVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.PhotonPersistentDisk == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("photon", volume.PhotonPersistentDisk.PdID)

	tags := map[string]string{
		"kind":  "photon",
		"pd-id": volume.PhotonPersistentDisk.PdID,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createPortWorxVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.PortworxVolume == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("portworx", volume.PortworxVolume.VolumeID)

	tags := map[string]string{
		"kind":      "portworx",
		"volume-id": volume.PortworxVolume.VolumeID,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createProjectedVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.Projected == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("projected", uuid.New())

	tags := map[string]string{
		"kind": "projection",
	}

	toCreate, err := vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
	if err != nil {
		return nil, err
	}

	for _, projection := range volume.Projected.Sources {
		if projection.ConfigMap != nil {
			cmExtID := vc.GetURNBuilder().BuildConfigMapExternalID(pod.Namespace, projection.ConfigMap.Name)

			toCreate.Relations = append(toCreate.Relations, projectedVolumeToProjectionStackStateRelation(vc, extID, cmExtID))
		} else if projection.Secret != nil {
			secExtID := vc.GetURNBuilder().BuildSecretExternalID(pod.Namespace, projection.Secret.Name)

			toCreate.Relations = append(toCreate.Relations, projectedVolumeToProjectionStackStateRelation(vc, extID, secExtID))
		} else if projection.DownwardAPI != nil {
			// Empty, nothing to do for downwardAPI
		}
		// TODO do we want to support ServiceAccount too?
	}

	return toCreate, nil
}

func createQuobyteVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.Quobyte == nil {
		return nil, nil
	}

	ids := []string{}
	for _, reg := range strings.Split(volume.Quobyte.Registry, ",") {
		ids = append(ids, vc.GetURNBuilder().BuildExternalVolumeExternalID("quobyte", reg, volume.Quobyte.Volume))
	}

	tags := map[string]string{
		"kind":     "quobyte",
		"volume":   volume.Quobyte.Volume,
		"registry": volume.Quobyte.Registry,
		"user":     volume.Quobyte.User,
	}

	extID := ids[0]
	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, ids[1:], tags)
}

func createRbdVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.RBD == nil {
		return nil, nil
	}

	ids := []string{}
	tags := map[string]string{
		"kind":  "rados",
		"pool":  volume.RBD.RBDPool,
		"image": volume.RBD.RBDImage,
	}

	for i, mon := range volume.RBD.CephMonitors {
		ids = append(ids, vc.GetURNBuilder().BuildExternalVolumeExternalID("rbd", mon, fmt.Sprintf("%s-image-%s", volume.RBD.RBDPool, volume.RBD.RBDImage)))
		tags[fmt.Sprintf("monitor-%d", i)] = mon
	}

	extID := ids[0]
	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, ids[1:], tags)
}

func createSecretVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.Secret == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildSecretExternalID(pod.Namespace, volume.Secret.SecretName)

	tags := map[string]string{
		"kind":       "secret",
		"secretName": volume.Secret.SecretName,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

// createScaleIoVolume DEPRECATED
func createScaleIoVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.ScaleIO == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("scale-io", volume.ScaleIO.Gateway, volume.ScaleIO.System)

	tags := map[string]string{
		"kind":              "scale-io",
		"gateway":           volume.ScaleIO.Gateway,
		"system":            volume.ScaleIO.System,
		"protection-domain": volume.ScaleIO.ProtectionDomain,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createStorageOsVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.StorageOS == nil {
		return nil, nil
	}

	ns := "default"
	if volume.StorageOS.VolumeNamespace != "" {
		ns = volume.StorageOS.VolumeNamespace
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("storage-os", ns, volume.StorageOS.VolumeName)

	tags := map[string]string{
		"kind":             "storage-os",
		"volume":           volume.StorageOS.VolumeName,
		"volume-namespace": volume.StorageOS.VolumeNamespace,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

func createVsphereVolume(vc VolumeCreator, pod PodIdentifier, volume v1.Volume) (*VolumeComponentsToCreate, error) {
	if volume.VsphereVolume == nil {
		return nil, nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("vsphere", volume.VsphereVolume.VolumePath)

	tags := map[string]string{
		"kind":           "vsphere",
		"volume-path":    volume.VsphereVolume.VolumePath,
		"storage-policy": volume.VsphereVolume.StoragePolicyName,
	}

	return vc.CreateStackStateVolumeSourceComponent(pod, volume, extID, nil, tags)
}

// Create a StackState relation from a Kubernetes / OpenShift Projected Volume to a Projection
func projectedVolumeToProjectionStackStateRelation(vc VolumeCreator, projectedVolumeExternalID, projectionExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes projected volume to projection relation: %s -> %s", projectedVolumeExternalID, projectionExternalID)

	relation := vc.CreateRelation(projectedVolumeExternalID, projectionExternalID, "projects")

	log.Tracef("Created StackState projected volume -> projection relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}
