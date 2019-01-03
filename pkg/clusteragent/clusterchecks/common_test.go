// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
)

type lockable interface {
	Lock()
	Unlock()
}

// requireNotLocked tries to lock a clusterStore to
// make sure it is not left locked after an operation
func requireNotLocked(t *testing.T, s *clusterStore) bool {
	t.Helper()
	if isLocked(s) {
		assert.FailNow(t, "clusterStore object is locked")
		return false
	}
	s.RLock()
	defer s.RUnlock()

	for node, store := range s.nodes {
		if isLocked(store) {
			assert.FailNowf(t, "nodeStore %s is locked", node)
			return false
		}
	}
	return true
}

func isLocked(l lockable) bool {
	ok := make(chan struct{}, 1)
	go func() {
		l.Lock()
		l.Unlock()
		ok <- struct{}{}
	}()
	select {
	case <-ok:
		return false
	case <-time.After(1 * time.Millisecond):
		return true
	}
}

func TestNotLocked(t *testing.T) {
	var m sync.Mutex
	assert.False(t, isLocked(&m))
}

func TestLocked(t *testing.T) {
	var m sync.Mutex
	m.Lock()

	assert.True(t, isLocked(&m))
}

func TestStoreNotLocked(t *testing.T) {
	s := newClusterStore()
	requireNotLocked(t, s)
}

type mockedPluggableAutoConfig struct {
	mock.Mock
}

func (m *mockedPluggableAutoConfig) AddScheduler(name string, s scheduler.Scheduler, replay bool) {
	m.Called(name, s, replay)
	return
}

func (m *mockedPluggableAutoConfig) RemoveScheduler(name string) {
	m.Called(name)
	return
}

// assertTrueBeforeTimeout regularly checks whether a condition is met. It
// does so until a timeout is reached, in which case it makes the test fail.
// Condition is evaluated in a goroutine to avoid tests hanging if a system
// is deadlocked.
func assertTrueBeforeTimeout(t *testing.T, frequency, timeout time.Duration, condition func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ok := make(chan struct{}, 1)
	var ranOnce bool

	go func() {
		// Try once immediately
		if condition() {
			ok <- struct{}{}
			return
		}

		// Retry until timeout
		checkTicker := time.NewTicker(frequency)
		defer checkTicker.Stop()
		for {
			select {
			case <-checkTicker.C:
				if condition() {
					ok <- struct{}{}
					return
				}
				ranOnce = true
			case <-ctx.Done():
				return
			}
		}
	}()

	select {
	case <-ok:
		return
	case <-ctx.Done():
		if ranOnce {
			assert.Fail(t, "Timeout waiting for condition to happen, function returned false")
		} else {
			assert.Fail(t, "Timeout waiting for condition to happen, function never returned")
		}
		return
	}
}
