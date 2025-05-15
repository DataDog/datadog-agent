// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !kubeapiserver

package loadstore

import "context"

// LocalAutoscalingWorkloadCheckResponse is a placeholder for the response type of the autoscaling workload check.
type LocalAutoscalingWorkloadCheckResponse []interface{}

// GetAutoscalingWorkloadCheck is a noop function that returns nil.
func GetAutoscalingWorkloadCheck(_ context.Context) *LocalAutoscalingWorkloadCheckResponse {
	return nil
}
