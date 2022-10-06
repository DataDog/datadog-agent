// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator
// +build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	corev1 "k8s.io/api/core/v1"
)

// ExtractNamespace returns the protobuf model corresponding to a Kubernetes Namespace resource.
func ExtractNamespace(ns *corev1.Namespace) *model.Namespace {
	return &model.Namespace{
		Metadata: extractMetadata(&ns.ObjectMeta),
		// status value based on https://github.com/kubernetes/kubernetes/blob/1e12d92a5179dbfeb455c79dbf9120c8536e5f9c/pkg/printers/internalversion/printers.go#L1350
		Status:           string(ns.Status.Phase),
		ConditionMessage: getNamespaceConditionMessage(ns),
	}
}

// getNamespaceConditionMessage loops through the namespace conditions, and reports the message of the first one
// (in the normal state transition order) that's doesn't pass
func getNamespaceConditionMessage(n *corev1.Namespace) string {
	messageMap := make(map[corev1.NamespaceConditionType]string)

	// from https://github.com/kubernetes/api/blob/master/core/v1/types.go#L5379-L5393
	// update if new ones appear
	// TODO !!!!!!! WHAT IS THE RIGHT ORDER?
	chronologicalConditions := []corev1.NamespaceConditionType{
		corev1.NamespaceDeletionDiscoveryFailure,
		corev1.NamespaceDeletionContentFailure,
		corev1.NamespaceDeletionGVParsingFailure,
		corev1.NamespaceContentRemaining,
		corev1.NamespaceFinalizersRemaining,
	}

	// populate messageMap with messages for non-passing conditions
	for _, c := range n.Status.Conditions {
		if c.Status == corev1.ConditionFalse && c.Message != "" {
			messageMap[c.Type] = c.Message
		}
	}

	// return the message of the first one that failed
	for _, c := range chronologicalConditions {
		if m := messageMap[c]; m != "" {
			return m
		}
	}
	return ""
}
