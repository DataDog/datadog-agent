// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	apiServerCommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewMutationFilter constructs an injection filter.
func NewMutationFilter(enabled bool, enabledNamespaces []string, disabledNamespaces []string) (MutationFilter, error) {
	filter, err := makeNamespaceFilter(enabledNamespaces, disabledNamespaces)

	mutationFilter := &mutationFilter{
		enabled: enabled,
		filter:  filter,
		err:     err,
	}

	return &injectionFilterImpl{NSFilter: mutationFilter}, err
}

type mutationFilter struct {
	enabled bool
	filter  *containers.Filter
	err     error
}

// IsNamespaceEligible returns true of APM Single Step Instrumentation
// is enabled and enabled for this namespace.
//
// There could be an error in configuration which would imply that
// APM is disabled.
//
// This DOES NOT respect `mutate_unlabelled` since it is a namespace
// specific check.
func (f *mutationFilter) IsNamespaceEligible(ns string) bool {
	if !f.enabled {
		log.Debugf("injection filter is disabled")
		return false
	}

	if f.err != nil {
		return false
	}

	if f.filter == nil {
		return false
	}

	return !f.filter.IsExcluded(nil, "", "", ns)
}

// Err returns an error if the namespace filter failed to initialize.
//
// This is safe to ignore for most uses, except for in auto_instrumentation itself.
func (f *mutationFilter) Err() error {
	return f.err
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
		return nil, fmt.Errorf("enabled_namespaces and disabled_namespaces configuration cannot be set together")
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

	disabledByDefault := []string{
		prefix + "^kube-system$",
		prefix + fmt.Sprintf("^%s$", apiServerCommon.GetResourcesNamespace()),
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
