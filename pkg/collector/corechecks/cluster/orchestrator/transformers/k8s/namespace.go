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

// ExtractNamespace returns the protobuf model corresponding to a Kubernetes Namespace resource.
func ExtractNamespace(ns *corev1.Namespace) *model.Namespace {
	n := &model.Namespace{
		Metadata: extractMetadata(&ns.ObjectMeta),
		// status value based on https://github.com/kubernetes/kubernetes/blob/1e12d92a5179dbfeb455c79dbf9120c8536e5f9c/pkg/printers/internalversion/printers.go#L1350
		Status:           string(ns.Status.Phase),
		ConditionMessage: getNamespaceConditionMessage(ns),
	}

	n.Tags = append(n.Tags, transformers.RetrieveUnifiedServiceTags(ns.ObjectMeta.Labels)...)

	return n
}

// getNamespaceConditionMessage loops through the namespace conditions, and reports the message of the first one
// (in the normal state transition order) that's doesn't pass
func getNamespaceConditionMessage(n *corev1.Namespace) string {
	messageMap := make(map[corev1.NamespaceConditionType]string)

	// from https://github.com/kubernetes/api/blob/master/core/v1/types.go#L5379-L5393
	// context https://github.com/kubernetes/design-proposals-archive/blob/8da1442ea29adccea40693357d04727127e045ed/architecture/namespaces.md#phases
	// update if new ones appear
	chronologicalConditions := []corev1.NamespaceConditionType{
		corev1.NamespaceContentRemaining,
		corev1.NamespaceFinalizersRemaining,
		corev1.NamespaceDeletionDiscoveryFailure,
		corev1.NamespaceDeletionGVParsingFailure,
		corev1.NamespaceDeletionContentFailure,
	}

	// populate messageMap with messages for non-passing conditions
	for _, c := range n.Status.Conditions {
		if c.Status == corev1.ConditionTrue && c.Message != "" {
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
