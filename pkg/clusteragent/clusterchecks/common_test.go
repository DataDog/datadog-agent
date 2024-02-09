// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
)

// requireNotLocked tries to lock a clusterStore to
// make sure it is not left locked after an operation
func requireNotLocked(t *testing.T, s *clusterStore) bool {
	t.Helper()
	if !s.TryRLock() {
		assert.FailNow(t, "clusterStore object is locked")
		return false
	}
	defer s.RUnlock()

	for node, store := range s.nodes {
		if !store.TryLock() {
			assert.FailNowf(t, "nodeStore %s is locked", node)
			return false
		}
		store.Unlock()
	}
	return true
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
}

func (m *mockedPluggableAutoConfig) RemoveScheduler(name string) {
	m.Called(name)
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
