// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package autoscaling implements the autoscaling controller
package autoscaling

import (
	corev1 "k8s.io/api/core/v1"
)

// team: container-integrations

// Component is the component type.
type Component interface {
	ApplyRecommendations(pod *corev1.Pod) (bool, error)
}
