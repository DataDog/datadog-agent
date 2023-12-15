// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWaitWithTimeoutTimesOut(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	// this will time out as wg.Done() is never called
	result := waitWithTimeout(&wg, 1*time.Millisecond)
	assert.Equal(t, result, true)
}

func TestWaitWithTimeoutCompletesNormally(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		wg.Done()
	}()
	result := waitWithTimeout(&wg, 250*time.Millisecond)
	assert.Equal(t, result, false)
}
