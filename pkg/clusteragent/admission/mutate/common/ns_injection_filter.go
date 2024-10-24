// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InjectionFilter is an interface to determine if a pod should be mutated.
type InjectionFilter interface {
	// ShouldMutatePod checks if a pod is mutable per explicit rules and
	// the NSFilter if InjectionFilter has one.
	ShouldMutatePod(pod *corev1.Pod) bool
	// IsNamespaceEligible returns true if a namespace is eligible for injection/mutation.
	IsNamespaceEligible(ns string) bool
	// InitError returns an error if the InjectionFilter failed to initialize.
	InitError() error
}

// NewInjectionFilter creates a new InjectionFilter with the given NamespaceInjectionFilter.
// the InjectionFilter encapsulates the logic for deciding whether
// we can do pod mutation based on a NSFilter (NamespaceInjectionFilter).
// See: InjectionFilter.ShouldMutatePod.
func NewInjectionFilter(filter NamespaceInjectionFilter) InjectionFilter {
	return &injectionFilterImpl{NSFilter: filter}
}

type injectionFilterImpl struct {
	NSFilter NamespaceInjectionFilter
}

// ShouldMutatePod checks if a pod is mutable per explicit rules and
// the NSFilter if InjectionFilter has one.
func (f injectionFilterImpl) ShouldMutatePod(pod *corev1.Pod) bool {
	switch getPodMutationLabelFlag(pod) {
	case podMutationDisabled:
		return false
	case podMutationEnabled:
		return true
	}

	if f.NSFilter != nil && f.NSFilter.IsNamespaceEligible(pod.Namespace) {
		return true
	}

	return pkgconfigsetup.Datadog().GetBool("admission_controller.mutate_unlabelled")
}

// IsNamespaceEligible returns true if a namespace is eligible for injection/mutation.
func (f injectionFilterImpl) IsNamespaceEligible(ns string) bool {
	return f.NSFilter.IsNamespaceEligible(ns)
}

// IsNamespaceEligible returns true if a namespace is eligible for injection/mutation.
func (f injectionFilterImpl) InitError() error {
	return f.NSFilter.Err()
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
	// Err returns an error if the InjectionFilter failed to initialize.
	Err() error
}
