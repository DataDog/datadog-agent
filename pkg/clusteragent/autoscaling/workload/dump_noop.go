// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !kubeapiserver

package workload

import (
	"io"
)

// AutoscalersInfo is an empty placeholder struct
type AutoscalersInfo struct{}

// Dump is a noop function that returns an empty AutoscalersInfo
func Dump() *AutoscalersInfo {
	return nil
}

// Print is a noop function that does nothing
func (*AutoscalersInfo) Print(io.Writer) {}
