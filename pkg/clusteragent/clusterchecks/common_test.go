// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
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
		// Ignore staticcheck SA2001.
		// This is a valid way of checking if l is locked. We will be able to
		// simplify all of this when we update to go 1.18, which includes
		// RWMutex.TryLock().
		//nolint:staticcheck
		l.Unlock()
		ok <- struct{}{}
	}()
	select {
	case <-ok:
		return false
	case <-time.After(1 * time.Second):
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

type fakeLeaderEngine struct {
	sync.Mutex
	ip  string
	err error
}

func (e *fakeLeaderEngine) get() (string, error) {
	e.Lock()
	defer e.Unlock()
	return e.ip, e.err
}

func (e *fakeLeaderEngine) set(ip string, err error) {
	e.Lock()
	defer e.Unlock()
	e.ip = ip
	e.err = err
}
