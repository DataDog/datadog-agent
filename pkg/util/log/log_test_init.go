// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package log

import (
	"os"
)

// TestLogLevel returns the level test binaries log at. It defaults to info, debug costing every
// emitted line a scrub and burying a failure's own output in unrelated noise.
func TestLogLevel() string {
	if level := os.Getenv("DD_LOG_LEVEL"); level != "" {
		return level
	}
	return InfoStr
}

func init() {
	SetupLogger(Default(), TestLogLevel())
}
