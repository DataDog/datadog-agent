// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagrules

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

// entityNamespace returns the entity's namespace; empty for cluster-scoped
// kinds.
func entityNamespace(entity *unstructured.Unstructured) string {
	return entity.GetNamespace()
}

// entityLabels returns the entity's metadata.labels map, possibly nil.
func entityLabels(entity *unstructured.Unstructured) map[string]string {
	return entity.GetLabels()
}

// asLabelSelector converts the rule selector to a labels.Selector. An empty
// selector matches everything.
func (s *Selector) asLabelSelector() (labels.Selector, error) {
	labelSelector := metav1.LabelSelector{
		MatchLabels:      s.MatchLabels,
		MatchExpressions: s.MatchExpressions,
	}
	selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		return nil, fmt.Errorf("building label selector: %w", err)
	}
	return selector, nil
}

// matches reports whether the entity matches the rule selector, including the
// namespace scope for pods.
func (s *Selector) matches(entity *unstructured.Unstructured, kind EntityKind) (bool, error) {
	selector, err := s.asLabelSelector()
	if err != nil {
		return false, err
	}
	if !selector.Matches(labels.Set(entityLabels(entity))) {
		return false, nil
	}
	if kind == EntityPod && s.Namespace != "" && s.Namespace != entityNamespace(entity) {
		return false, nil
	}
	return true, nil
}

// ruleMatchesEntity reports whether the rule targets the entity's kind and
// matches it by selector.
func ruleMatchesEntity(rule *TagRule, entity *unstructured.Unstructured, kind EntityKind) (bool, error) {
	if rule.Spec.Entity != kind {
		return false, nil
	}
	return rule.Spec.Selector.matches(entity, kind)
}
