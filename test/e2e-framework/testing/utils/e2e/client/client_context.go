// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

// Context is the subset of common.Context needed by the client layer.
// It has no dependency on *testing.T, so non-test callers can implement it.
type Context interface {
	Logf(format string, args ...any)
	// FailNow records the formatted message and immediately stops the current goroutine.
	// In test contexts this emits a structured Testify assertion; in other contexts it logs and panics.
	FailNow(format string, args ...any)
	SessionOutputDir() string
}
