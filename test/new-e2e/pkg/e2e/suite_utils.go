// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import "testing"

type testLogger struct {
	t *testing.T
}

func newTestLogger(t *testing.T) testLogger {
	return testLogger{t: t}
}

func (tl testLogger) Write(p []byte) (n int, err error) {
	tl.t.Log(string(p))
	return len(p), nil
}
