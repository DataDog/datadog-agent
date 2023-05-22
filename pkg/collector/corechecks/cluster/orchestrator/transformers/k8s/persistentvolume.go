// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"strings"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"

	corev1 "k8s.io/api/core/v1"
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

	message.Spec.PersistentVolumeType = extractVolumeSource(pv.Spec.PersistentVolumeSource)

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

func extractVolumeSource(volume corev1.PersistentVolumeSource) string {
	switch {
	case volume.HostPath != nil:
		return "HostPath"
	case volume.GCEPersistentDisk != nil:
		return "GCEPersistentDisk"
	case volume.AWSElasticBlockStore != nil:
		return "AWSElasticBlockStore"
	case volume.Quobyte != nil:
		return "Quobyte"
	case volume.Cinder != nil:
		return "Cinder"
	case volume.PhotonPersistentDisk != nil:
		return "PhotonPersistentDisk"
	case volume.PortworxVolume != nil:
		return "PortworxVolume"
	case volume.ScaleIO != nil:
		return "ScaleIO"
	case volume.CephFS != nil:
		return "CephFS"
	case volume.StorageOS != nil:
		return "StorageOS"
	case volume.FC != nil:
		return "FC"
	case volume.AzureFile != nil:
		return "AzureFile"
	case volume.FlexVolume != nil:
		return "FlexVolume"
	case volume.Flocker != nil:
		return "Flocker"
	case volume.CSI != nil:
		return "CSI"
	}
	return "<unknown>"
}
