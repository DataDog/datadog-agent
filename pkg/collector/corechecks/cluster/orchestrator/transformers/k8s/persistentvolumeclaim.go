// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"

	corev1 "k8s.io/api/core/v1"
)

// ExtractPersistentVolumeClaim returns the protobuf model corresponding to a
// Kubernetes PersistentVolumeClaim resource.
func ExtractPersistentVolumeClaim(pvc *corev1.PersistentVolumeClaim) *model.PersistentVolumeClaim {
	message := &model.PersistentVolumeClaim{
		Metadata: extractMetadata(&pvc.ObjectMeta),
		Spec: &model.PersistentVolumeClaimSpec{
			VolumeName: pvc.Spec.VolumeName,
			Resources:  &model.ResourceRequirements{},
		},
		Status: &model.PersistentVolumeClaimStatus{
			Phase:    string(pvc.Status.Phase),
			Capacity: map[string]int64{},
		},
	}
	extractSpec(pvc, message)
	extractStatus(pvc, message)
	message.Tags = append(message.Tags, transformers.RetrieveUnifiedServiceTags(pvc.ObjectMeta.Labels)...)

	return message
}

func extractSpec(pvc *corev1.PersistentVolumeClaim, message *model.PersistentVolumeClaim) {
	ds := pvc.Spec.DataSource
	if ds != nil {
		t := &model.TypedLocalObjectReference{Kind: ds.Kind, Name: ds.Name}
		if ds.APIGroup != nil {
			t.ApiGroup = *ds.APIGroup
		}
		message.Spec.DataSource = t
	}

	if pvc.Spec.VolumeMode != nil {
		message.Spec.VolumeMode = string(*pvc.Spec.VolumeMode)
	}

	if pvc.Spec.StorageClassName != nil {
		message.Spec.StorageClassName = *pvc.Spec.StorageClassName
	}

	if pvc.Spec.AccessModes != nil {
		strModes := make([]string, len(pvc.Spec.AccessModes))
		for i, am := range pvc.Spec.AccessModes {
			strModes[i] = string(am)
		}
		message.Spec.AccessModes = strModes
	}

	reSt := pvc.Spec.Resources.Requests.Storage()
	if reSt != nil && !reSt.IsZero() {
		message.Spec.Resources.Requests = map[string]int64{string(corev1.ResourceStorage): reSt.Value()}
	}
	reLt := pvc.Spec.Resources.Limits.Storage()
	if reLt != nil && !reLt.IsZero() {
		message.Spec.Resources.Limits = map[string]int64{string(corev1.ResourceStorage): reLt.Value()}
	}

	if pvc.Spec.Selector != nil {
		message.Spec.Selector = extractLabelSelector(pvc.Spec.Selector)
	}
}

func extractStatus(pvc *corev1.PersistentVolumeClaim, message *model.PersistentVolumeClaim) {
	pvcCons := pvc.Status.Conditions
	if len(pvcCons) > 0 {
		cons := make([]*model.PersistentVolumeClaimCondition, len(pvcCons))
		for i, condition := range pvcCons {
			cons[i] = &model.PersistentVolumeClaimCondition{
				Type:    string(condition.Type),
				Status:  string(condition.Status),
				Reason:  condition.Reason,
				Message: condition.Message,
			}
			if !condition.LastProbeTime.IsZero() {
				cons[i].LastProbeTime = condition.LastProbeTime.Unix()
			}
			if !condition.LastTransitionTime.IsZero() {
				cons[i].LastTransitionTime = condition.LastProbeTime.Unix()
			}
		}
		message.Status.Conditions = cons
	}

	if pvc.Status.AccessModes != nil {
		strModes := make([]string, len(pvc.Status.AccessModes))
		for i, am := range pvc.Status.AccessModes {
			strModes[i] = string(am)
		}
		message.Status.AccessModes = strModes
	}

	st := pvc.Status.Capacity.Storage()
	if st != nil && !st.IsZero() {
		message.Status.Capacity[corev1.ResourceStorage.String()] = st.Value()
	}
}
