// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package ebpftest

import "testing"

// LogTracePipe is unsupported
func LogTracePipe(t *testing.T) {
}

// LogTracePipeSelf is unsupported
func LogTracePipeSelf(t *testing.T) {
}

// LogTracePipeProcess is unsupported
func LogTracePipeProcess(t *testing.T, pid uint32) {
}

// LogTracePipeFilter is unsupported
func LogTracePipeFilter(t *testing.T, filterFn func(ev *TraceEvent) bool) {
}
