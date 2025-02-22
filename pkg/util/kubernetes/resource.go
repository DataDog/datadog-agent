// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver || kubelet

package kubernetes

import (
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// FormatCPURequests converts a CPU request to a percentage of a CPU core pointer
// For 100Mi, AsApproximate returns 0.1, we return 10%
func FormatCPURequests(cpuRequest resource.Quantity) *float64 {
	return pointer.Ptr(cpuRequest.AsApproximateFloat64() * 100)
}

// FormatMemoryRequests converts a memory request to a uint64 pointer
func FormatMemoryRequests(memoryRequest resource.Quantity) *uint64 {
	return pointer.Ptr(uint64(memoryRequest.Value()))
}
