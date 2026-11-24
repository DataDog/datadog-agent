// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package schedulerevents

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

// newTestCheck returns a check wired to a mock sender and to the given leader
// answer, as Configure leaves it, without a workloadmeta subscription.
func newTestCheck(t *testing.T, leader func() (bool, error)) (*Check, *mocksender.MockSender) {
	t.Helper()

	sender := mocksender.NewMockSender(t, CheckName)
	sender.SetupAcceptAll()

	c := &Check{
		CheckBase: core.NewCheckBase(CheckName),
		leader:    leader,
		startedAt: started,
	}
	require.NoError(t, c.CommonConfigure(sender.GetSenderManager(), nil, nil, "", ""))

	submit := func(e event.Event) { c.send(e) }
	c.kueue = newKueueEvents(fakeTagger{}, newTestIndex(), started, submit)
	c.kueue.now = func() time.Time { return after(1000) }
	return c, sender
}

func setKueue(w *kueueView) workloadmeta.Event {
	w.SchedulerEvents = w.KueueWorkloadEvents
	return workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: w.KubernetesKueueWorkload}
}

func TestRunSendsEventsOfQueuedUpdatesWhenLeader(t *testing.T) {
	c, sender := newTestCheck(t, func() (bool, error) { return true, nil })
	c.queue = []workloadmeta.Event{setKueue(kueueWorkload(after(5)))}

	require.NoError(t, c.Run())

	sender.AssertNumberOfCalls(t, "Event", 1)
	sender.AssertNumberOfCalls(t, "Commit", 1)
	assert.Empty(t, c.queue, "the queue is drained")
}

func TestRunKeepsStateButSendsNothingWhenFollower(t *testing.T) {
	leader := false
	c, sender := newTestCheck(t, func() (bool, error) { return leader, nil })

	c.queue = []workloadmeta.Event{setKueue(kueueWorkload(after(5)))}
	require.NoError(t, c.Run())
	sender.AssertNumberOfCalls(t, "Event", 0)
	require.Contains(t, c.kueue.states, "uid-1")

	// After taking the lead, only what changed since is reported, not the
	// creation that the previous leader already reported.
	leader = true
	c.queue = []workloadmeta.Event{setKueue(kueueWorkload(after(5), kueueCondition("Finished", "True", "Succeeded", after(30))))}
	require.NoError(t, c.Run())
	sender.AssertNumberOfCalls(t, "Event", 1)
}

func TestIsLeaderIsFalseWithoutErrorWhenLeaderElectionIsOff(t *testing.T) {
	cfg := configmock.NewMockWithOverrides(t, map[string]interface{}{"leader_election": false})

	leader, err := isLeader(cfg)

	require.NoError(t, err)
	assert.False(t, leader)
}

func TestRunHandlesUpdatesAsAFollowerWhenTheLeaderIsUnknown(t *testing.T) {
	failure := errors.New("no leader election")
	c, sender := newTestCheck(t, func() (bool, error) { return false, failure })
	c.queue = []workloadmeta.Event{setKueue(kueueWorkload(after(5)))}

	err := c.Run()

	require.ErrorIs(t, err, failure, "the run is reported as failed")
	sender.AssertNumberOfCalls(t, "Event", 0)
	assert.Empty(t, c.queue, "the queue is drained, so it cannot grow while the leader is unknown")
	assert.Contains(t, c.kueue.states, "uid-1", "the state is recorded")

	// Once the leader is known, only what changes after is sent: the creation
	// was recorded and is not reported.
	c.leader = func() (bool, error) { return true, nil }
	c.queue = []workloadmeta.Event{setKueue(kueueWorkload(after(5), kueueCondition("Finished", "True", "Succeeded", after(30))))}
	require.NoError(t, c.Run())
	sender.AssertNumberOfCalls(t, "Event", 1)
	assert.Empty(t, c.queue)
}

func TestDedupeTagsKeepsFirstOccurrences(t *testing.T) {
	assert.Equal(t, []string{"a:1", "b:2", "c:3"}, dedupeTags([]string{"a:1", "b:2", "a:1", "c:3", "b:2"}))
	assert.Empty(t, dedupeTags(nil))
}

func TestRunForgetsUnsetWorkloads(t *testing.T) {
	c, _ := newTestCheck(t, func() (bool, error) { return true, nil })
	w := kueueWorkload(after(5))
	c.queue = []workloadmeta.Event{setKueue(w)}
	require.NoError(t, c.Run())
	require.Contains(t, c.kueue.states, "uid-1")

	c.queue = []workloadmeta.Event{{Type: workloadmeta.EventTypeUnset, Entity: w.KubernetesKueueWorkload}}
	require.NoError(t, c.Run())
	assert.NotContains(t, c.kueue.states, "uid-1")
}

func TestRunIgnoresEntitiesOfDisabledScheduler(t *testing.T) {
	c, sender := newTestCheck(t, func() (bool, error) { return true, nil })
	require.Nil(t, c.kai)
	c.queue = []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: &workloadmeta.KubernetesKAIPodGroup{
		EntityMeta: workloadmeta.EntityMeta{UID: "pg-1"},
	}}}

	require.NoError(t, c.Run())
	sender.AssertNumberOfCalls(t, "Event", 0)
}

func TestReceiveQueuesAndAcknowledgesBundles(t *testing.T) {
	c, _ := newTestCheck(t, func() (bool, error) { return true, nil })
	updates := make(chan workloadmeta.EventBundle)
	done := make(chan struct{})
	go func() { c.receive(updates); close(done) }()

	ack := make(chan struct{})
	updates <- workloadmeta.EventBundle{Ch: ack, Events: []workloadmeta.Event{setKueue(kueueWorkload(after(5)))}}
	select {
	case <-ack:
	case <-time.After(5 * time.Second):
		t.Fatal("bundle not acknowledged")
	}
	close(updates)
	<-done

	c.mu.Lock()
	defer c.mu.Unlock()
	assert.Len(t, c.queue, 1)
}

func TestConfigureSkipsInstanceWhenEventsAreOff(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]interface{}
	}{
		{"all off", map[string]interface{}{}},
		{"events on but integration off", map[string]interface{}{
			"cluster_agent.kueue.events.enabled":         true,
			"cluster_agent.kai_scheduler.events.enabled": true,
		}},
		{"integration on but events off", map[string]interface{}{
			"cluster_agent.kueue.enabled":         true,
			"cluster_agent.kai_scheduler.enabled": true,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := configmock.NewMockWithOverrides(t, test.overrides)
			sender := mocksender.NewMockSender(t, CheckName)
			factory := Factory(nil, nil, cfg)
			newCheck, ok := factory.Get()
			require.True(t, ok)
			c := newCheck().(*Check)

			err := c.Configure(sender.GetSenderManager(), 0, nil, nil, "", "")

			assert.ErrorIs(t, err, check.ErrSkipCheckInstance)
			assert.Nil(t, c.updates, "no subscription for a skipped instance")
		})
	}
}

// fakeWorkloadmeta implements the subscription part of workloadmeta.
type fakeWorkloadmeta struct {
	workloadmeta.Component
	updates      chan workloadmeta.EventBundle
	filter       *workloadmeta.Filter
	subscribers  []string
	unsubscribed bool
}

func (f *fakeWorkloadmeta) Subscribe(name string, _ workloadmeta.SubscriberPriority, filter *workloadmeta.Filter) chan workloadmeta.EventBundle {
	f.subscribers = append(f.subscribers, name)
	f.filter = filter
	f.updates = make(chan workloadmeta.EventBundle)
	return f.updates
}

func (f *fakeWorkloadmeta) Unsubscribe(ch chan workloadmeta.EventBundle) {
	f.unsubscribed = true
	close(ch)
}

func TestConfigureSubscribesToTheKindsWithEventsOn(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]interface{}
		kinds     []workloadmeta.Kind
		kueue     bool
		kai       bool
	}{
		{
			name: "kueue only",
			overrides: map[string]interface{}{
				"cluster_agent.kueue.enabled":        true,
				"cluster_agent.kueue.events.enabled": true,
			},
			kinds: []workloadmeta.Kind{workloadmeta.KindKubernetesKueueWorkload},
			kueue: true,
		},
		{
			name: "kai only",
			overrides: map[string]interface{}{
				"cluster_agent.kai_scheduler.enabled":        true,
				"cluster_agent.kai_scheduler.events.enabled": true,
			},
			kinds: []workloadmeta.Kind{workloadmeta.KindKubernetesKAIPodGroup},
			kai:   true,
		},
		{
			name: "both",
			overrides: map[string]interface{}{
				"cluster_agent.kueue.enabled":                true,
				"cluster_agent.kueue.events.enabled":         true,
				"cluster_agent.kai_scheduler.enabled":        true,
				"cluster_agent.kai_scheduler.events.enabled": true,
			},
			kinds: []workloadmeta.Kind{workloadmeta.KindKubernetesKueueWorkload, workloadmeta.KindKubernetesKAIPodGroup},
			kueue: true,
			kai:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wmeta := &fakeWorkloadmeta{}
			cfg := configmock.NewMockWithOverrides(t, test.overrides)
			sender := mocksender.NewMockSender(t, CheckName)
			factory := Factory(wmeta, nil, cfg)
			newCheck, ok := factory.Get()
			require.True(t, ok)
			c := newCheck().(*Check)
			c.leader = func() (bool, error) { return true, nil }

			require.NoError(t, c.Configure(sender.GetSenderManager(), 0, nil, nil, "", ""))

			assert.Equal(t, []string{CheckName}, wmeta.subscribers)
			assert.ElementsMatch(t, test.kinds, wmeta.filter.Kinds())
			assert.Equal(t, test.kueue, c.kueue != nil)
			assert.Equal(t, test.kai, c.kai != nil)

			// A bundle sent by the store is acknowledged and reaches the next run.
			ack := make(chan struct{})
			wmeta.updates <- workloadmeta.EventBundle{Ch: ack, Events: []workloadmeta.Event{setKueue(kueueWorkload(after(5)))}}
			select {
			case <-ack:
			case <-time.After(5 * time.Second):
				t.Fatal("bundle not acknowledged")
			}
			c.mu.Lock()
			assert.Len(t, c.queue, 1)
			c.mu.Unlock()

			c.Cancel()
			assert.True(t, wmeta.unsubscribed)
		})
	}
}
