// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package autodiscovery

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MockProvider struct {
	collectCounter int
}

func (p *MockProvider) Collect() ([]integration.Config, error) {
	p.collectCounter++
	return []integration.Config{}, nil
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

type MockListener struct {
	ListenCount  int
	stopReceived bool
}

func (l *MockListener) Listen(newSvc, delSvc chan<- listeners.Service) { l.ListenCount++ }
func (l *MockListener) Stop()                                          { l.stopReceived = true }

func TestAddProvider(t *testing.T) {
	ac := NewAutoConfig(scheduler.NewMetaScheduler())
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

func TestAddListener(t *testing.T) {
	ac := NewAutoConfig(scheduler.NewMetaScheduler())
	assert.Len(t, ac.listeners, 0)
	ml := &MockListener{}
	ac.AddListener(ml)
	require.Len(t, ac.listeners, 1)
	assert.Equal(t, 1, ml.ListenCount)
}

func TestContains(t *testing.T) {
	c1 := integration.Config{Name: "bar"}
	c2 := integration.Config{Name: "foo"}
	pd := providerDescriptor{}
	pd.configs = append(pd.configs, c1)
	assert.True(t, pd.contains(&c1))
	assert.False(t, pd.contains(&c2))
}

func TestStop(t *testing.T) {
	ac := NewAutoConfig(scheduler.NewMetaScheduler())
	ac.StartPolling() // otherwise Stop would block

	ml := &MockListener{}
	ac.AddListener(ml)

	ac.Stop()

	assert.True(t, ml.stopReceived)
	assert.True(t, ml.stopReceived)
}
