// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package config

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

// NewFilter creates a new MutationFilter from the provided FilterConfig.
func NewFilter(datadogConfig config.Component) (mutatecommon.MutationFilter, error) {
	enabled := datadogConfig.GetBool("admission_controller.inject_config.enabled")
	return mutatecommon.NewDefaultFilter(enabled, nil, nil)
}
