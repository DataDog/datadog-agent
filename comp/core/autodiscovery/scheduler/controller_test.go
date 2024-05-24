// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package scheduler

import (
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func makeConfig(name string) integration.Config {
	return integration.Config{
		Name: name,
	}
}

type event struct {
	isSchedule bool
	configName string
}

type scheduler struct {
	events []event
	mutex  sync.Mutex
}

func (s *scheduler) Schedule(configs []integration.Config) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for _, config := range configs {
		s.events = append(s.events, event{true, config.Name})
	}
}

func (s *scheduler) Unschedule(configs []integration.Config) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for _, config := range configs {
		s.events = append(s.events, event{false, config.Name})
	}
}

func (s *scheduler) Stop() {}

func (s *scheduler) reset() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.events = []event{}
}

func TestController(t *testing.T) {
	ms := NewController()

	// schedule some configs before registering
	c1 := makeConfig("one")
	c2 := makeConfig("two")
	ms.ApplyChanges(integration.ConfigChanges{Schedule: []integration.Config{c1, c2}})
	// register a scheduler and see that it gets caught up
	s1 := &scheduler{}
	ms.Register("s1", s1, true)
	s1.mutex.Lock()
	require.ElementsMatch(t, []event{{true, "one"}, {true, "two"}}, s1.events)
	s1.mutex.Unlock()

	ms.ApplyChanges(integration.ConfigChanges{Schedule: []integration.Config{c1, c2}})
	clk := clock.NewMock()
	clk.Add(2000 * time.Millisecond)
	s1.reset()
	// remove one of those configs and add another
	ms.ApplyChanges(integration.ConfigChanges{Unschedule: []integration.Config{c1}})
	c3 := makeConfig("three")
	ms.ApplyChanges(integration.ConfigChanges{Schedule: []integration.Config{c3}})
	// check s1 was informed about those in order
	clk.Add(2000 * time.Millisecond)
	s1.mutex.Lock()
	require.Equal(t, []event{{false, "one"}, {true, "three"}}, s1.events)
	s1.mutex.Unlock()
	s1.reset()

	// subscribe a new scheduler and see that it does not get c1
	s2 := &scheduler{}
	ms.Register("s2", s2, true)
	clk.Add(2000 * time.Millisecond)
	s2.mutex.Lock()
	require.ElementsMatch(t, []event{{true, "two"}, {true, "three"}}, s2.events)
	s2.mutex.Unlock()
	s2.reset()

	// unsubscribe s1 and see that it no longer gets events
	ms.Deregister("s1")
	ms.ApplyChanges(integration.ConfigChanges{Unschedule: []integration.Config{c2}})
	clk.Add(2000 * time.Millisecond)
	s1.mutex.Lock()
	require.Equal(t, []event{}, s1.events)
	s1.mutex.Unlock()
	s1.reset()
	s2.mutex.Lock()
	require.Equal(t, []event{{false, "two"}}, s2.events)
	s2.mutex.Unlock()
	s2.reset()

	// verify that replay does not occur if not desired
	s3 := &scheduler{}
	ms.Register("s3", s3, false)
	s3.mutex.Lock()
	require.ElementsMatch(t, []event{}, s3.events)
	s3.mutex.Unlock()
	ms.Stop()
	s3.reset()
}
