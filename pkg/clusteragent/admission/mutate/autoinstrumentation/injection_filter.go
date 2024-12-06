// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	apiServerCommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewInjectionFilter constructs an injection filter using the autoinstrumentation
// GetNamespaceInjectionFilter.
func NewInjectionFilter(datadogConfig config.Component) (mutatecommon.InjectionFilter, error) {
	filter, err := makeAPMSSINamespaceFilter(
		datadogConfig.GetStringSlice("apm_config.instrumentation.enabled_namespaces"),
		datadogConfig.GetStringSlice("apm_config.instrumentation.disabled_namespaces"),
	)

	injectionFilter := &injectionFilter{
		apmInstrumentationEnabled: datadogConfig.GetBool("apm_config.instrumentation.enabled"),
		filter:                    filter,

		err: err,
	}
	return mutatecommon.NewInjectionFilter(injectionFilter), err
}

type injectionFilter struct {
	apmInstrumentationEnabled bool

	filter *containers.Filter
	err    error
}

// IsNamespaceEligible returns true of APM Single Step Instrumentation
// is enabled and enabled for this namespace.
//
// There could be an error in configuration which would imply that
// APM is disabled.
//
// This DOES NOT respect `mutate_unlabelled` since it is a namespace
// specific check.
func (f *injectionFilter) IsNamespaceEligible(ns string) bool {
	if !f.apmInstrumentationEnabled {
		log.Debugf("APM Instrumentation is disabled")
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
func (f *injectionFilter) Err() error {
	return f.err
}

// makeAPMSSINamespaceFilter returns the filter used by APM SSI to filter namespaces.
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
func makeAPMSSINamespaceFilter(apmEnabledNamespaces, apmDisabledNamespaces []string) (*containers.Filter, error) {
	if len(apmEnabledNamespaces) > 0 && len(apmDisabledNamespaces) > 0 {
		return nil, fmt.Errorf("apm.instrumentation.enabled_namespaces and apm.instrumentation.disabled_namespaces configuration cannot be set together")
	}

	// Prefix the namespaces as needed by the containers.Filter.
	prefix := containers.KubeNamespaceFilterPrefix
	apmEnabledNamespacesWithPrefix := make([]string, len(apmEnabledNamespaces))
	apmDisabledNamespacesWithPrefix := make([]string, len(apmDisabledNamespaces))

	for i := range apmEnabledNamespaces {
		apmEnabledNamespacesWithPrefix[i] = prefix + fmt.Sprintf("^%s$", apmEnabledNamespaces[i])
	}
	for i := range apmDisabledNamespaces {
		apmDisabledNamespacesWithPrefix[i] = prefix + fmt.Sprintf("^%s$", apmDisabledNamespaces[i])
	}

	disabledByDefault := []string{
		prefix + "^kube-system$",
		prefix + fmt.Sprintf("^%s$", apiServerCommon.GetResourcesNamespace()),
	}

	var filterExcludeList []string
	if len(apmEnabledNamespacesWithPrefix) > 0 && len(apmDisabledNamespacesWithPrefix) == 0 {
		// In this case, we want to include only the namespaces in the enabled list.
		// In the containers.Filter, the include list is checked before the
		// exclude list, that's why we set the exclude list to all namespaces.
		filterExcludeList = []string{prefix + ".*"}
	} else {
		filterExcludeList = append(apmDisabledNamespacesWithPrefix, disabledByDefault...)
	}

	return containers.NewFilter(containers.GlobalFilter, apmEnabledNamespacesWithPrefix, filterExcludeList)
}
