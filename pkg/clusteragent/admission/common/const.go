// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

const (
	// EnabledLabelKey pod label to disable/enable mutations at the pod level.
	EnabledLabelKey = "admission.datadoghq.com/enabled"

	// InjectionModeLabelKey pod label to chose the config injection at the pod level.
	InjectionModeLabelKey = "admission.datadoghq.com/config.mode"

	// LibVersionAnnotKeyFormat is the format of the library version annotation
	LibVersionAnnotKeyFormat = "admission.datadoghq.com/%s-lib.version"

	// LibConfigV1AnnotKeyFormat is the format of the library config annotation
	LibConfigV1AnnotKeyFormat = "admission.datadoghq.com/%s-lib.config.v1"
)
