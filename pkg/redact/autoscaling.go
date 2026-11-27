// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
)

// ScrubHorizontalPodAutoscaler scrubs the metadata of a HorizontalPodAutoscaler.
func ScrubHorizontalPodAutoscaler(r *autoscalingv2.HorizontalPodAutoscaler, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}
