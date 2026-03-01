// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package jmx

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSetStartupErrorConcurrent exercises SetStartupError and GetStartupError
// concurrently. With the race detector enabled (-race) this test would have
// caught the original bug where SetStartupError incorrectly held
// lastJMXStatusMutex instead of lastJMXStartupErrorMutex.
func TestSetStartupErrorConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			SetStartupError(StartupError{LastError: "err", Timestamp: 1})
		}()
		go func() {
			defer wg.Done()
			_ = GetStartupError()
		}()
	}
	wg.Wait()

	SetStartupError(StartupError{LastError: "final", Timestamp: 42})
	got := GetStartupError()
	assert.Equal(t, "final", got.LastError)
	assert.Equal(t, int64(42), got.Timestamp)
}
