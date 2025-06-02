// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package common

// ScaleDirection represents the scaling direction
type ScaleDirection string

const (
	// NoScale indicates no scaling action is needed
	NoScale ScaleDirection = "noScale"
	// ScaleUp indicates scaling up is needed
	ScaleUp ScaleDirection = "scaleUp"
	// ScaleDown indicates scaling down is needed
	ScaleDown ScaleDirection = "scaleDown"
)

// GetScaleDirection gets the scaling direction based on the current number of replicas vs the recommendation
func GetScaleDirection(currentReplicas, recommendedReplicas int32) ScaleDirection {
	if currentReplicas < recommendedReplicas {
		return ScaleUp
	} else if currentReplicas > recommendedReplicas {
		return ScaleDown
	}
	return NoScale
}
