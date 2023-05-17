// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// extractMetadata extracts standard metadata into the protobuf model.
func extractMetadata(m *metav1.ObjectMeta) *model.Metadata {
	meta := model.Metadata{
		Name:            m.Name,
		Namespace:       m.Namespace,
		Uid:             string(m.UID),
		ResourceVersion: m.ResourceVersion,
	}
	if !m.CreationTimestamp.IsZero() {
		meta.CreationTimestamp = m.CreationTimestamp.Unix()
	}
	if !m.DeletionTimestamp.IsZero() {
		meta.DeletionTimestamp = m.DeletionTimestamp.Unix()
	}
	if len(m.Annotations) > 0 {
		meta.Annotations = mapToTags(m.Annotations)
	}
	if len(m.Labels) > 0 {
		meta.Labels = mapToTags(m.Labels)
	}
	if len(m.Finalizers) > 0 {
		meta.Finalizers = m.Finalizers
	}
	for _, o := range m.OwnerReferences {
		owner := model.OwnerReference{
			Name: o.Name,
			Uid:  string(o.UID),
			Kind: o.Kind,
		}
		meta.OwnerReferences = append(meta.OwnerReferences, &owner)
	}

	return &meta
}

func extractLabelSelector(ls *metav1.LabelSelector) []*model.LabelSelectorRequirement {
	labelSelectors := make([]*model.LabelSelectorRequirement, 0, len(ls.MatchLabels)+len(ls.MatchExpressions))
	for k, v := range ls.MatchLabels {
		s := model.LabelSelectorRequirement{
			Key:      k,
			Operator: "In",
			Values:   []string{v},
		}
		labelSelectors = append(labelSelectors, &s)
	}
	for _, s := range ls.MatchExpressions {
		sr := model.LabelSelectorRequirement{
			Key:      s.Key,
			Operator: string(s.Operator),
			Values:   s.Values,
		}
		labelSelectors = append(labelSelectors, &sr)
	}

	return labelSelectors
}
