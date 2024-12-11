// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"
)

// ExtractPersistentVolume returns the protobuf model corresponding to a Kubernetes
// PersistentVolume resource.
func ExtractPersistentVolume(pv *corev1.PersistentVolume) *model.PersistentVolume {
	message := &model.PersistentVolume{
		Metadata: extractMetadata(&pv.ObjectMeta),
		Spec: &model.PersistentVolumeSpec{
			Capacity:                      map[string]int64{},
			PersistentVolumeReclaimPolicy: string(pv.Spec.PersistentVolumeReclaimPolicy),
			StorageClassName:              pv.Spec.StorageClassName,
			MountOptions:                  pv.Spec.MountOptions,
		},
		Status: &model.PersistentVolumeStatus{
			Phase:   string(pv.Status.Phase),
			Message: pv.Status.Message,
			Reason:  pv.Status.Reason,
		},
	}

	if pv.Spec.VolumeMode != nil {
		message.Spec.VolumeMode = string(*pv.Spec.VolumeMode)
	}

	modes := pv.Spec.AccessModes
	if len(modes) > 0 {
		ams := make([]string, len(modes))
		for i, mode := range modes {
			ams[i] = string(mode)
		}
		message.Spec.AccessModes = ams
	}

	claimRef := pv.Spec.ClaimRef
	if claimRef != nil {
		message.Spec.ClaimRef = &model.ObjectReference{
			Kind:            claimRef.Kind,
			Namespace:       claimRef.Namespace,
			Name:            claimRef.Name,
			Uid:             string(claimRef.UID),
			ApiVersion:      claimRef.APIVersion,
			ResourceVersion: claimRef.ResourceVersion,
			FieldPath:       claimRef.FieldPath,
		}
	}

	nodeAffinity := pv.Spec.NodeAffinity
	if nodeAffinity != nil {
		selectorTerms := make([]*model.NodeSelectorTerm, len(nodeAffinity.Required.NodeSelectorTerms))
		terms := nodeAffinity.Required.NodeSelectorTerms
		for i, term := range terms {
			selectorTerms[i] = &model.NodeSelectorTerm{
				MatchExpressions: extractPVSelector(term.MatchExpressions),
				MatchFields:      extractPVSelector(term.MatchFields),
			}
		}
		message.Spec.NodeAffinity = selectorTerms
	}

	addVolumeSource(message, pv.Spec.PersistentVolumeSource)

	st := pv.Spec.Capacity.Storage()
	if !st.IsZero() {
		message.Spec.Capacity[corev1.ResourceStorage.String()] = st.Value()
	}

	addAdditionalPersistentVolumeTags(message)

	message.Tags = append(message.Tags, transformers.RetrieveUnifiedServiceTags(pv.ObjectMeta.Labels)...)

	return message
}

func addAdditionalPersistentVolumeTags(pvModel *model.PersistentVolume) {
	additionalTags := []string{
		"pv_phase:" + strings.ToLower(pvModel.Status.Phase),
		"pv_type:" + strings.ToLower(pvModel.Spec.PersistentVolumeType),
	}
	pvModel.Tags = append(pvModel.Tags, additionalTags...)
}

func extractPVSelector(ls []corev1.NodeSelectorRequirement) []*model.LabelSelectorRequirement {
	if len(ls) == 0 {
		return nil
	}

	labelSelectors := make([]*model.LabelSelectorRequirement, len(ls))
	for i, v := range ls {
		labelSelectors[i] = &model.LabelSelectorRequirement{
			Key:      v.Key,
			Operator: string(v.Operator),
			Values:   v.Values,
		}
	}
	return labelSelectors
}

func addVolumeSource(pvModel *model.PersistentVolume, volume corev1.PersistentVolumeSource) {
	switch {
	case volume.Local != nil:
		pvModel.Spec.PersistentVolumeType = "LocalVolume"
	case volume.HostPath != nil:
		pvModel.Spec.PersistentVolumeType = "HostPath"
	case volume.GCEPersistentDisk != nil:
		pvModel.Spec.PersistentVolumeType = "GCEPersistentDisk"
		pvModel.Spec.PersistentVolumeSource = &model.PersistentVolumeSource{
			GcePersistentDisk: extractGCEPersistentDiskVolumeSource(volume),
		}
	case volume.AWSElasticBlockStore != nil:
		pvModel.Spec.PersistentVolumeType = "AWSElasticBlockStore"
		pvModel.Spec.PersistentVolumeSource = &model.PersistentVolumeSource{
			AwsElasticBlockStore: extractAWSElasticBlockStoreVolumeSource(volume),
		}
	case volume.Quobyte != nil:
		pvModel.Spec.PersistentVolumeType = "Quobyte"
	case volume.Cinder != nil:
		pvModel.Spec.PersistentVolumeType = "Cinder"
	case volume.PhotonPersistentDisk != nil:
		pvModel.Spec.PersistentVolumeType = "PhotonPersistentDisk"
	case volume.PortworxVolume != nil:
		pvModel.Spec.PersistentVolumeType = "PortworxVolume"
	case volume.ScaleIO != nil:
		pvModel.Spec.PersistentVolumeType = "ScaleIO"
	case volume.CephFS != nil:
		pvModel.Spec.PersistentVolumeType = "CephFS"
	case volume.StorageOS != nil:
		pvModel.Spec.PersistentVolumeType = "StorageOS"
	case volume.FC != nil:
		pvModel.Spec.PersistentVolumeType = "FC"
	case volume.AzureFile != nil:
		pvModel.Spec.PersistentVolumeType = "AzureFile"
		pvModel.Spec.PersistentVolumeSource = &model.PersistentVolumeSource{
			AzureFile: extractAzureFilePersistentVolumeSource(volume),
		}
	case volume.AzureDisk != nil:
		pvModel.Spec.PersistentVolumeType = "AzureDisk"
		pvModel.Spec.PersistentVolumeSource = &model.PersistentVolumeSource{
			AzureDisk: extractAzureDiskVolumeSource(volume),
		}
	case volume.FlexVolume != nil:
		pvModel.Spec.PersistentVolumeType = "FlexVolume"
	case volume.Flocker != nil:
		pvModel.Spec.PersistentVolumeType = "Flocker"
	case volume.CSI != nil:
		pvModel.Spec.PersistentVolumeType = "CSI"
		pvModel.Spec.PersistentVolumeSource = &model.PersistentVolumeSource{
			Csi: extractCSIVolumeSource(volume),
		}
	default:
		pvModel.Spec.PersistentVolumeType = "<unknown>"
	}
}

func extractGCEPersistentDiskVolumeSource(volume corev1.PersistentVolumeSource) *model.GCEPersistentDiskVolumeSource {
	return &model.GCEPersistentDiskVolumeSource{
		PdName:    volume.GCEPersistentDisk.PDName,
		FsType:    volume.GCEPersistentDisk.FSType,
		Partition: volume.GCEPersistentDisk.Partition,
		ReadOnly:  volume.GCEPersistentDisk.ReadOnly,
	}
}

func extractAWSElasticBlockStoreVolumeSource(volume corev1.PersistentVolumeSource) *model.AWSElasticBlockStoreVolumeSource {
	return &model.AWSElasticBlockStoreVolumeSource{
		VolumeID:  volume.AWSElasticBlockStore.VolumeID,
		FsType:    volume.AWSElasticBlockStore.FSType,
		Partition: volume.AWSElasticBlockStore.Partition,
		ReadOnly:  volume.AWSElasticBlockStore.ReadOnly,
	}
}

func extractAzureFilePersistentVolumeSource(volume corev1.PersistentVolumeSource) *model.AzureFilePersistentVolumeSource {
	m := &model.AzureFilePersistentVolumeSource{
		SecretName: volume.AzureFile.SecretName,
		ShareName:  volume.AzureFile.ShareName,
		ReadOnly:   volume.AzureFile.ReadOnly,
	}
	if volume.AzureFile.SecretNamespace != nil {
		m.SecretNamespace = *volume.AzureFile.SecretNamespace
	}
	return m
}

func extractAzureDiskVolumeSource(volume corev1.PersistentVolumeSource) *model.AzureDiskVolumeSource {
	m := &model.AzureDiskVolumeSource{
		DiskName: volume.AzureDisk.DiskName,
		DiskURI:  volume.AzureDisk.DataDiskURI,
	}
	if volume.AzureDisk.CachingMode != nil {
		m.CachingMode = string(*volume.AzureDisk.CachingMode)
	}
	if volume.AzureDisk.FSType != nil {
		m.FsType = *volume.AzureDisk.FSType
	}
	if volume.AzureDisk.ReadOnly != nil {
		m.ReadOnly = *volume.AzureDisk.ReadOnly
	}
	if volume.AzureDisk.Kind != nil {
		m.Kind = string(*volume.AzureDisk.Kind)
	}

	return m
}

func extractCSIVolumeSource(volume corev1.PersistentVolumeSource) *model.CSIVolumeSource {
	m := &model.CSIVolumeSource{
		Driver:           volume.CSI.Driver,
		VolumeHandle:     volume.CSI.VolumeHandle,
		ReadOnly:         volume.CSI.ReadOnly,
		FsType:           volume.CSI.FSType,
		VolumeAttributes: volume.CSI.VolumeAttributes,
	}

	m.ControllerPublishSecretRef = extractSecretReference(volume.CSI.ControllerPublishSecretRef)
	m.NodeStageSecretRef = extractSecretReference(volume.CSI.NodeStageSecretRef)
	m.NodePublishSecretRef = extractSecretReference(volume.CSI.NodePublishSecretRef)
	m.ControllerExpandSecretRef = extractSecretReference(volume.CSI.ControllerExpandSecretRef)
	return m
}

func extractSecretReference(ref *corev1.SecretReference) *model.SecretReference {
	if ref == nil {
		return nil
	}
	return &model.SecretReference{
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}
}
