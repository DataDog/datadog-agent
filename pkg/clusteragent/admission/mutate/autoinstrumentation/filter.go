// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

// NewFilter creates a new MutationFilter from the provided FilterConfig.
func NewFilter(config *Config) (mutatecommon.MutationFilter, error) {
	return mutatecommon.NewDefaultFilter(
		config.Instrumentation.Enabled,
		config.Instrumentation.EnabledNamespaces,
		config.Instrumentation.DisabledNamespaces)
}
