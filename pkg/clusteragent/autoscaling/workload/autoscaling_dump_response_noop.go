// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !kubeapiserver

package workload

import (
	"io"
)

// AutoscalingDumpResponse is an empty placeholder struct
type AutoscalingDumpResponse struct{}

// Dump is a noop function that returns an empty AutoscalingDumpResponse
func Dump() *AutoscalingDumpResponse {
	return nil
}

// Write is a noop function that does nothing
func (*AutoscalingDumpResponse) Write(io.Writer) {}
