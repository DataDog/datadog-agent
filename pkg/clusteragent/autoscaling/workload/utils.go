// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"cmp"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

func min[T cmp.Ordered](a T, b T) T {
	if a < b {
		return a
	}
	return b
}

func max[T cmp.Ordered](a T, b T) T {
	if a > b {
		return a
	}
	return b
}

// applyModeAllowsSource returns true if the given source is allowed by the given apply mode.
func applyModeAllowsSource(mode datadoghq.DatadogPodAutoscalerApplyMode, _ datadoghqcommon.DatadogPodAutoscalerValueSource) bool {
	switch mode {
	case datadoghq.DatadogPodAutoscalerApplyModeApply:
		return true
	default:
		return false
	}
}
