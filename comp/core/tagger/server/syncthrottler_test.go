// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"sync"
	"testing"
	"time"
)

func TestSyncThrottler(_ *testing.T) {

	throtler := NewSyncThrottler(3)

	var wg sync.WaitGroup

	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := throtler.RequestToken()
			time.Sleep(200 * time.Millisecond)
			throtler.Release(t)
			throtler.Release(t) // Release method should be idempotent
		}()
	}

	wg.Wait()
}
