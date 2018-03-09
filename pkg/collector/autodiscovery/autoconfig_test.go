// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package autodiscovery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/listeners"
)

type MockProvider struct {
	collectCounter int
}

func (p *MockProvider) Collect() ([]check.Config, error) {
	p.collectCounter++
	return []check.Config{}, nil
}

func (p *MockProvider) String() string {
	return "mocked"
}

func (p *MockProvider) IsUpToDate() (bool, error) {
	return true, nil
}

type MockProvider2 struct {
	MockProvider
}

type MockLoader struct{}

func (l *MockLoader) Load(config check.Config) ([]check.Check, error) { return []check.Check{}, nil }

type MockListener struct {
	stopReceived bool
	started      chan struct{}
}

func (l *MockListener) Listen(newSvc, delSvc chan<- listeners.Service) {
	l.started <- struct{}{}
}

func (l *MockListener) Stop() {
	l.stopReceived = true
}

func TestAddProvider(t *testing.T) {
	ac := NewAutoConfig(nil)
	ac.StartPolling()
	assert.Len(t, ac.providers, 0)
	mp := &MockProvider{}
	ac.AddProvider(mp, false)
	ac.AddProvider(mp, false) // this should be a noop
	ac.AddProvider(&MockProvider2{}, true)
	ac.LoadAndRun()
	require.Len(t, ac.providers, 2)
	assert.Equal(t, 1, mp.collectCounter)
	assert.False(t, ac.providers[0].poll)
	assert.True(t, ac.providers[1].poll)
}

func TestAddLoader(t *testing.T) {
	ac := NewAutoConfig(nil)
	assert.Len(t, ac.loaders, 0)
	ac.AddLoader(&MockLoader{})
	ac.AddLoader(&MockLoader{}) // noop
	assert.Len(t, ac.loaders, 1)
}

func TestAddListener(t *testing.T) {
	ac := NewAutoConfig(nil)
	assert.Len(t, ac.listeners, 0)
	ml := &MockListener{
		started: make(chan struct{}, 1),
	}
	ac.AddListener(ml)
	select {
	case <-ml.started:
		// Cool
	case <-time.After(100 * time.Millisecond):
		assert.FailNow(t, "listener didn't start in 100 ms")
	}
	require.Len(t, ac.listeners, 1)
}

func TestContains(t *testing.T) {
	c1 := check.Config{Name: "bar"}
	c2 := check.Config{Name: "foo"}
	pd := providerDescriptor{}
	pd.configs = append(pd.configs, c1)
	assert.True(t, pd.contains(&c1))
	assert.False(t, pd.contains(&c2))
}

func TestStop(t *testing.T) {
	ac := NewAutoConfig(nil)
	ac.StartPolling() // otherwise Stop would block
	ml := &MockListener{
		started: make(chan struct{}, 1),
	}
	ac.AddListener(ml)
	select {
	case <-ml.started:
		// Cool
	case <-time.After(100 * time.Millisecond):
		assert.FailNow(t, "listener didn't start in 100 ms")
	}

	ac.Stop()
	assert.True(t, ml.stopReceived)
}
