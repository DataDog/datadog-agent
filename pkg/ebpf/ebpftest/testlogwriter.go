// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"testing"
)

// TestLogWriter wraps the testing.T object and provides a simple
// io.Writer interface, to be used with DumpMaps functions
// Very simple implementation now for output in debug functions so
// newlines aren't handled: each call to Write is just sent to
// t.Log
type TestLogWriter struct {
	T *testing.T
}

// Write method implementation, sends the data to t.Log()
func (tlw *TestLogWriter) Write(p []byte) (int, error) {
	tlw.T.Log(string(p))

	return len(p), nil
}
