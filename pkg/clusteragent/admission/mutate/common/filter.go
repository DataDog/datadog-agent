// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MutationFilter is an interface to determine if a pod should be mutated.
type MutationFilter interface {
	// ShouldMutatePod checks if a pod is mutable per explicit rules and
	// the NSFilter if InjectionFilter has one.
	ShouldMutatePod(pod *corev1.Pod) bool
	// IsNamespaceEligible returns true if a namespace is eligible for injection/mutation.
	IsNamespaceEligible(ns string) bool
}

// DefaultDisabledNamespaces returns the default namespaces that are disabled for injection/mutation.
func DefaultDisabledNamespaces() []string {
	return []string{
		"kube-system",
		namespace.GetResourcesNamespace(),
	}
}

// DefaultFilter provides a default implementation of the MutationFilter interface that uses namespaces for filtering.
type DefaultFilter struct {
	enabled bool
	filter  *containers.Filter
}

// NewDefaultFilter constructs the default mutation filter from the enabled flag and the list of enabled and disabled
// namespaces.
func NewDefaultFilter(enabled bool, enabledNamespaces []string, disabledNamespaces []string) (*DefaultFilter, error) {
	filter, err := makeNamespaceFilter(enabledNamespaces, disabledNamespaces)
	return &DefaultFilter{
		enabled: enabled,
		filter:  filter,
	}, err
}

// ShouldMutatePod checks if a pod is mutable per explicit rules and them validates the namespace.
func (f *DefaultFilter) ShouldMutatePod(pod *corev1.Pod) bool {
	switch getPodMutationLabelFlag(pod) {
	case podMutationDisabled:
		return false
	case podMutationEnabled:
		return true
	}

	if f.IsNamespaceEligible(pod.Namespace) {
		return true
	}

	return pkgconfigsetup.Datadog().GetBool("admission_controller.mutate_unlabelled")
}

// IsNamespaceEligible returns true if a namespace is eligible for injection/mutation.
func (f *DefaultFilter) IsNamespaceEligible(ns string) bool {
	if !f.enabled {
		log.Debugf("injection filter is disabled")
		return false
	}

	if f.filter == nil {
		return false
	}

	return !f.filter.IsExcluded(nil, "", "", ns)
}

// makeNamespaceFilter returns a filter with the provided enabled/disabled namespaces.
// The filter excludes two namespaces by default: "kube-system" and the
// namespace where datadog is installed.
//
// Cases:
//   - No enabled namespaces and no disabled namespaces: inject in all namespaces
//     except the 2 namespaces excluded by default.
//   - Enabled namespaces and no disabled namespaces: inject only in the
//     namespaces specified in the list of enabled namespaces. If one of the
//     namespaces excluded by default is included in the list, it will be injected.
//   - Disabled namespaces and no enabled namespaces: inject only in the
//     namespaces that are not included in the list of disabled namespaces and that
//     are not one of the ones disabled by default.
//   - Enabled and disabled namespaces: return error.
func makeNamespaceFilter(enabledNamespaces, disabledNamespaces []string) (*containers.Filter, error) {
	if len(enabledNamespaces) > 0 && len(disabledNamespaces) > 0 {
		return nil, errors.New("enabled_namespaces and disabled_namespaces configuration cannot be set together")
	}

	// Prefix the namespaces as needed by the containers.Filter.
	prefix := containers.KubeNamespaceFilterPrefix
	enabledNamespacesWithPrefix := make([]string, len(enabledNamespaces))
	disabledNamespacesWithPrefix := make([]string, len(disabledNamespaces))

	for i := range enabledNamespaces {
		enabledNamespacesWithPrefix[i] = prefix + fmt.Sprintf("^%s$", enabledNamespaces[i])
	}
	for i := range disabledNamespaces {
		disabledNamespacesWithPrefix[i] = prefix + fmt.Sprintf("^%s$", disabledNamespaces[i])
	}

	defaultDisabled := DefaultDisabledNamespaces()
	disabledByDefault := make([]string, len(defaultDisabled))
	for i := range defaultDisabled {
		disabledByDefault[i] = prefix + fmt.Sprintf("^%s$", defaultDisabled[i])
	}

	var filterExcludeList []string
	if len(enabledNamespacesWithPrefix) > 0 && len(disabledNamespacesWithPrefix) == 0 {
		// In this case, we want to include only the namespaces in the enabled list.
		// In the containers.Filter, the include list is checked before the
		// exclude list, that's why we set the exclude list to all namespaces.
		filterExcludeList = []string{prefix + ".*"}
	} else {
		filterExcludeList = append(disabledNamespacesWithPrefix, disabledByDefault...)
	}

	return containers.NewFilter(containers.GlobalFilter, enabledNamespacesWithPrefix, filterExcludeList)
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
