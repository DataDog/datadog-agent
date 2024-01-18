// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flake

import (
	"math/rand"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockTesting struct {
	*testing.T

	mutex          sync.RWMutex
	skipCallCount  int
	errorCallCount int
	logs           []any
}

func newMockTesting(t *testing.T) *mockTesting {
	return &mockTesting{
		T: t,
	}
}

func (mt *mockTesting) Skip(_ ...any) {
	func() {
		mt.mutex.Lock()
		defer mt.mutex.Unlock()
		mt.skipCallCount++
	}()
	// implement testing.T.Skip() call to runtime.Goexit()
	// to mock the behavior of testing.T.Skip()
	runtime.Goexit()
}

func (mt *mockTesting) Errorf(_ string, _ ...any) {
	mt.mutex.Lock()
	defer mt.mutex.Unlock()
	mt.errorCallCount++
}

func (mt *mockTesting) SkipCount() int {
	mt.mutex.RLock()
	defer mt.mutex.RUnlock()
	return mt.skipCallCount
}

func (mt *mockTesting) ErrorCount() int {
	mt.mutex.RLock()
	defer mt.mutex.RUnlock()
	return mt.errorCallCount
}

func (mt *mockTesting) Log(args ...any) {
	mt.mutex.Lock()
	defer mt.mutex.Unlock()
	mt.logs = append(mt.logs, args)
}

var (
	trueValue  = true
	falseValue = false
)

func TestFlake(t *testing.T) {
	t.Run("skip flake test", func(t *testing.T) {
		mt := newMockTesting(t)
		skipFlake = &trueValue
		wrapAndRunFlakyTest(mt)
		assert.Equal(t, mt.SkipCount(), 1)
		assert.Equal(t, 0, mt.ErrorCount())
	})
	t.Run("mark flake test", func(t *testing.T) {
		mt := newMockTesting(t)
		skipFlake = &falseValue
		wrapAndRunFlakyTest(mt)
		assert.Equal(t, mt.logs, []any{[]any{flakyTestMessage}})
		assert.Greater(t, mt.ErrorCount(), 1)
		assert.Equal(t, 0, mt.SkipCount())
	})
}

func wrapAndRunFlakyTest(t *mockTesting) {
	t.Helper()
	wg := sync.WaitGroup{}
	// testing.T.Skip() calls runtime.Goexit() which terminates the goroutine
	// run the test in a separate goroutine to avoid terminating `TestFlake` test
	wg.Add(1)
	go func() {
		defer wg.Done()
		Mark(t)
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				coin := flipCoin()
				assert.Equal(t, "heads", coin)
			}()
		}
	}()
	wg.Wait()
}

func flipCoin() string {
	if rand.Intn(2) == 1 {
		return "heads"
	}
	return "tails"
}
