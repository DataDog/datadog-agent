// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
)

// ApplyModeAllowSource returns true if the given source is allowed by the given apply mode.
func ApplyModeAllowSource(mode datadoghq.DatadogPodAutoscalerApplyMode, source datadoghq.DatadogPodAutoscalerValueSource) bool {
	switch mode {
	case datadoghq.DatadogPodAutoscalerAllApplyNone:
	case datadoghq.DatadogPodAutoscalerManualApplyMode:
		return source == datadoghq.DatadogPodAutoscalerManualValueSource
	case datadoghq.DatadogPodAutoscalerAllApplyMode:
		return true
	default:
	}
	return false
}
