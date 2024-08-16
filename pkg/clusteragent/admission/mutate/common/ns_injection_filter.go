// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InjectionFilter encapsulates the logic for deciding whether
// we can do pod mutation based on a NSFilter (NamespaceInjectionFilter).
// See: InjectionFilter.ShouldMutatePod.
type InjectionFilter struct {
	NSFilter NamespaceInjectionFilter
}

// ShouldMutatePod checks if a pod is mutable per explicit rules and
// the NSFilter if InjectionFilter has one.
func (f InjectionFilter) ShouldMutatePod(pod *corev1.Pod) bool {
	switch getPodMutationLabelFlag(pod) {
	case podMutationDisabled:
		return false
	case podMutationEnabled:
		return true
	}

	if f.NSFilter != nil && f.NSFilter.IsNamespaceEligible(pod.Namespace) {
		return true
	}

	return config.Datadog().GetBool("admission_controller.mutate_unlabelled")
}

type podMutationLabelFlag int

const (
	podMutationUnspecified podMutationLabelFlag = iota
	podMutationEnabled
	podMutationDisabled
)

// getPodMutationLabelFlag returns podMutationUnspecified if the label is not
// set or if the label is set to an invalid value.
func getPodMutationLabelFlag(pod *corev1.Pod) podMutationLabelFlag {
	if val, found := pod.GetLabels()[common.EnabledLabelKey]; found {
		switch val {
		case "true":
			return podMutationEnabled
		case "false":
			return podMutationDisabled
		default:
			log.Warnf(
				"Invalid label value '%s=%s' on pod %s should be either 'true' or 'false', ignoring it",
				common.EnabledLabelKey,
				val,
				PodString(pod),
			)
		}
	}

	return podMutationUnspecified
}

// NamespaceInjectionFilter represents a contract to be able to filter out which pods are
// eligible for mutation/injection.
//
// This exists to separate the direct implementation in the autoinstrumentation package and
// its dependencies in other webhooks.
//
// See autoinstrumentation.GetInjectionFilter.
type NamespaceInjectionFilter interface {
	// IsNamespaceEligible returns true if a namespace is eligible for injection/mutation.
	IsNamespaceEligible(ns string) bool
	// Err returns an error if creation of the NamespaceInjectionFilter failed.
	Err() error
}
