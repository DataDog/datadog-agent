package topologycollectors

import (
	"fmt"
	"strings"

	"github.com/pborman/uuid"
	v1 "k8s.io/api/core/v1"
)

// VolumeSourceMapper maps a VolumeSource to an external Volume topology component externalID
type VolumeSourceMapper func(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error)

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

func createAwsEbsVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.AWSElasticBlockStore == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("aws-ebs", volume.AWSElasticBlockStore.VolumeID, fmt.Sprint(volume.AWSElasticBlockStore.Partition))
	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createAzureDiskVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.AzureDisk == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("azure-disk", volume.AzureDisk.DiskName)

	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createAzureFileVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.AzureFile == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("azure-file", volume.AzureFile.ShareName)

	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createCephFsVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.CephFS == nil {
		return "", nil
	}

	components := func(idx int) []string {
		c := []string{volume.CephFS.Monitors[idx]}
		if volume.CephFS.Path != "" {
			c = append(c, volume.CephFS.Path)
		}
		return c
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("ceph-fs", components(0)...)

	idx := 1
	identifiers := []string{}

	for idx < len(volume.CephFS.Monitors) {
		identifiers = append(identifiers, vc.GetURNBuilder().BuildExternalVolumeExternalID("ceph-fs", components(idx)...))
		idx++
	}

	return vc.createStackStateVolumeComponent(pod, volume, extID, identifiers)
}

func createCinderVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.Cinder == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("cinder", volume.Cinder.VolumeID)

	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createConfigMapVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.ConfigMap == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildConfigMapExternalID(pod.Namespace, volume.ConfigMap.Name)

	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createEmptyDirVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.EmptyDir == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildVolumeExternalID("empty-dir", fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, volume.Name))

	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createFCVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.FC == nil {
		return "", nil
	}

	ids := []string{}
	if len(volume.FC.TargetWWNs) > 0 {
		for _, wwn := range volume.FC.TargetWWNs {
			ids = append(ids, vc.GetURNBuilder().BuildExternalVolumeExternalID("fibre-channel", fmt.Sprintf("%s-lun-%d", wwn, *volume.FC.Lun)))
		}
	} else if len(volume.FC.WWIDs) > 0 {
		for _, wwid := range volume.FC.WWIDs {
			ids = append(ids, vc.GetURNBuilder().BuildExternalVolumeExternalID("fibre-channel", wwid))
		}
	} else {
		return "", fmt.Errorf("Either volume.FC.TargetWWNs or volume.FC.WWIDs needs to be set")
	}

	extID := ids[0]
	identifiers := ids[1:]
	return vc.createStackStateVolumeComponent(pod, volume, extID, identifiers)
}

func createFlexVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.FlexVolume == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("flex", volume.FlexVolume.Driver)
	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

// createFlockerVolume DEPRECATED
func createFlockerVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.Flocker == nil {
		return "", nil
	}

	var extID string
	if volume.Flocker.DatasetName != "" {
		extID = vc.GetURNBuilder().BuildExternalVolumeExternalID("flocker", volume.Flocker.DatasetName)
	} else {
		extID = vc.GetURNBuilder().BuildExternalVolumeExternalID("flocker", volume.Flocker.DatasetUUID)
	}
	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createGcePersistentDiskVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.GCEPersistentDisk == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("gce-pd", volume.GCEPersistentDisk.PDName)
	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

// createGitRepoVolume DEPRECATED
func createGitRepoVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.GitRepo == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("git-repo", volume.GitRepo.Repository)
	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createGlusterFsVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.Glusterfs == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("gluster-fs", volume.Glusterfs.EndpointsName, volume.Glusterfs.Path)
	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createHostPathVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.HostPath == nil {
		return "", nil
	} else if pod.NodeName == "" { // Not scheduled yet...
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("hostpath", pod.NodeName, volume.HostPath.Path)
	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createIscsiVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.ISCSI == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("iscsi", volume.ISCSI.TargetPortal, volume.ISCSI.IQN, fmt.Sprint(volume.ISCSI.Lun))

	identifiers := []string{}
	for _, tp := range volume.ISCSI.Portals {
		identifiers = append(identifiers, vc.GetURNBuilder().BuildExternalVolumeExternalID("iscsi", tp, volume.ISCSI.IQN, fmt.Sprint(volume.ISCSI.Lun)))
	}

	return vc.createStackStateVolumeComponent(pod, volume, extID, identifiers)
}

func createNfsVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.NFS == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("nfs", volume.NFS.Server, volume.NFS.Path)

	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createPhotonPersistentDiskVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.PhotonPersistentDisk == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("photon", volume.PhotonPersistentDisk.PdID)

	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createPortWorxVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.PortworxVolume == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("portworx", volume.PortworxVolume.VolumeID)

	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createProjectedVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.Projected == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("projected", uuid.New())

	_, err := vc.createStackStateVolumeComponent(pod, volume, extID, nil)
	if err != nil {
		return "", err
	}

	for _, projection := range volume.Projected.Sources {
		if projection.ConfigMap != nil {
			cmExtID := vc.GetURNBuilder().BuildConfigMapExternalID(pod.Namespace, projection.ConfigMap.Name)

			vc.RelationChan <- vc.projectedVolumeToProjectionStackStateRelation(extID, cmExtID)
		} else if projection.Secret != nil {
			secExtID := vc.GetURNBuilder().BuildSecretExternalID(pod.Namespace, projection.Secret.Name)

			vc.RelationChan <- vc.projectedVolumeToProjectionStackStateRelation(extID, secExtID)
		} else if projection.DownwardAPI != nil {
			// Empty, nothing to do for downwardAPI
		}
		// TODO do we want to support ServiceAccount too?
	}

	return extID, nil
}

func createQuobyteVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.Quobyte == nil {
		return "", nil
	}

	ids := []string{}
	for _, reg := range strings.Split(volume.Quobyte.Registry, ",") {
		ids = append(ids, vc.GetURNBuilder().BuildExternalVolumeExternalID("quobyte", reg, volume.Quobyte.Volume))
	}

	extID := ids[0]
	return vc.createStackStateVolumeComponent(pod, volume, extID, ids[1:])
}

func createRbdVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.RBD == nil {
		return "", nil
	}

	ids := []string{}
	for _, mon := range volume.RBD.CephMonitors {
		ids = append(ids, vc.GetURNBuilder().BuildExternalVolumeExternalID("rbd", mon, fmt.Sprintf("%s-image-%s", volume.RBD.RBDPool, volume.RBD.RBDImage)))
	}

	extID := ids[0]
	return vc.createStackStateVolumeComponent(pod, volume, extID, ids[1:])
}

func createSecretVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.Secret == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildSecretExternalID(pod.Namespace, volume.Secret.SecretName)

	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

// createScaleIoVolume DEPRECATED
func createScaleIoVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.ScaleIO == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("scale-io", volume.ScaleIO.Gateway, volume.ScaleIO.System)

	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createStorageOsVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.StorageOS == nil {
		return "", nil
	}

	ns := "default"
	if volume.StorageOS.VolumeNamespace != "" {
		ns = volume.StorageOS.VolumeNamespace
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("storage-os", ns, volume.StorageOS.VolumeName)
	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}

func createVsphereVolume(vc *VolumeCorrelator, pod PodIdentifier, volume v1.Volume) (string, error) {
	if volume.VsphereVolume == nil {
		return "", nil
	}

	extID := vc.GetURNBuilder().BuildExternalVolumeExternalID("vsphere", volume.VsphereVolume.VolumePath)
	return vc.createStackStateVolumeComponent(pod, volume, extID, nil)
}
