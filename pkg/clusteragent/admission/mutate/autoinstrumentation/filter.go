// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

// FilterConfig provides a struct to configure the filter.
type FilterConfig struct {
	enabled            bool
	enabledNamespaces  []string
	disabledNamespaces []string
}

// NewFilterConfig creates a new FilterConfig from the datadogConfig.
func NewFilterConfig(datadogConfig config.Component) *FilterConfig {
	return &FilterConfig{
		enabled:            datadogConfig.GetBool("apm_config.instrumentation.enabled"),
		enabledNamespaces:  datadogConfig.GetStringSlice("apm_config.instrumentation.enabled_namespaces"),
		disabledNamespaces: datadogConfig.GetStringSlice("apm_config.instrumentation.disabled_namespaces"),
	}
}

// NewFilter creates a new MutationFilter from the provided FilterConfig.
func NewFilter(cfg *FilterConfig) (mutatecommon.MutationFilter, error) {
	return mutatecommon.NewDefaulFilter(cfg.enabled, cfg.enabledNamespaces, cfg.disabledNamespaces)
}
