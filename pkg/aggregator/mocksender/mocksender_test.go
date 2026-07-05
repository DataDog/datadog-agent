// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package mocksender

import (
	"testing"
	"testing/synctest"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// TestNewMockSenderNoGoroutineLeak verifies that NewMockSender leaves no
// goroutines behind after the test ends. synctest.Test deadlocks if any
// goroutine started in the bubble is still running when the test function
// returns and t.Cleanup has run.
func TestNewMockSenderNoGoroutineLeak(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_ = NewMockSender(t, checkid.ID("test"))
	})
}
