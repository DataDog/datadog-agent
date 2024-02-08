// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package ebpftest is utilities for tests against eBPF
package ebpftest

import "testing"

// LogTracePipe is unsupported
func LogTracePipe(*testing.T) {
}

// LogTracePipeSelf is unsupported
func LogTracePipeSelf(*testing.T) {
}

// LogTracePipeProcess is unsupported
func LogTracePipeProcess(_ *testing.T, pid uint32) { //nolint:revive // TODO fix revive unused-parameter
}

// LogTracePipeFilter is unsupported
func LogTracePipeFilter(*testing.T, func(ev *TraceEvent) bool) {
}
