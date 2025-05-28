// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !kubeapiserver

/*
Package loadstore provides a noop implementation for the autoscaling workload check.
*/
package loadstore

import (
	"context"
	"io"
)

// LoadstoreMetricInfo is a placeholder for the response type of the autoscaling workload check.
type LoadstoreMetricInfo struct{}

// GetAutoscalingWorkloadCheck is a noop function that returns nil.
func GetAutoscalingWorkloadCheck(_ context.Context) *LoadstoreMetricInfo {
	return nil
}

// Dump is a noop function.
func (ls *LoadstoreMetricInfo) Dump(w io.Writer) {
	// No-op
}
