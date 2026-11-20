// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package schedulerevents implements the check that emits the events of the
// batch schedulers (Kueue and KAI Scheduler) from the Cluster Agent.
//
// The Cluster Agent already has every Kueue Workload and KAI PodGroup in its
// workloadmeta, so the check sees each change as it happens and does not need
// to list them again. Only the leader of the Cluster Agent sends the events,
// so there is no rule to pick one instance among several.
package schedulerevents

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "job_scheduler_events"

	kueueEventIndexCacheKey = CheckName + ":kueue_workload_event_index"
	kaiEventIndexCacheKey   = CheckName + ":kai_pod_group_event_index"

	// pruneAfter is how long the check waits before it drops the event counts of
	// Workloads and PodGroups that do not exist anymore. The Workloads and
	// PodGroups arrive after the check starts, so it cannot tell sooner which
	// ones are gone.
	pruneAfter = 2 * time.Minute
)

var emittedEvents = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
	CheckName,
	"emitted_events",
	[]string{"scheduler", "transition"},
	"Number of scheduler events emitted by the check.",
	telemetry.Options{NoDoubleUnderscoreSep: true},
)

// Check emits the events of Kueue Workloads and KAI PodGroups.
type Check struct {
	core.CheckBase

	wmeta  workloadmeta.Component
	tagger tagger.Component
	cfg    config.Component

	startedAt time.Time
	pruned    bool

	kueue *kueueEvents
	kai   *kaiEvents

	// events subscription. A goroutine confirms each bundle as soon as it
	// arrives, since the store does not notify the next subscriber until then,
	// and queues its events for the next run.
	updates chan workloadmeta.EventBundle
	mu      sync.Mutex
	queue   []workloadmeta.Event

	// sender is the sender of the run in progress.
	sender sender.Sender

	// leader tells whether this replica is the leader. It is a field so that
	// tests do not need a leader election.
	leader func() (bool, error)
}

// Factory creates a new check factory
func Factory(wmeta workloadmeta.Component, tagger tagger.Component, cfg config.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return &Check{
			CheckBase: core.NewCheckBase(CheckName),
			wmeta:     wmeta,
			tagger:    tagger,
			cfg:       cfg,
			leader:    func() (bool, error) { return isLeader(cfg) },
		}
	})
}

// Configure subscribes the check to the Workloads and PodGroups it has events for.
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string, provider string) error {
	c.BuildID(integrationConfigDigest, config, initConfig)
	if err := c.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return err
	}

	kueueEnabled := c.cfg.GetBool("cluster_agent.kueue.enabled") && c.cfg.GetBool("cluster_agent.kueue.events.enabled")
	kaiEnabled := c.cfg.GetBool("cluster_agent.kai_scheduler.enabled") && c.cfg.GetBool("cluster_agent.kai_scheduler.events.enabled")
	if !kueueEnabled && !kaiEnabled {
		return check.ErrSkipCheckInstance
	}

	if !c.cfg.GetBool("leader_election") {
		log.Warn("leader_election is not enabled in the Cluster Agent, the " + CheckName + " check will not send events")
	}

	c.startedAt = time.Now()
	submit := func(e event.Event) { c.send(e) }

	filter := workloadmeta.NewFilterBuilder()
	if kueueEnabled {
		c.kueue = newKueueEvents(c.tagger, newEventIndex(kueueEventIndexCacheKey), c.startedAt, submit)
		filter = filter.AddKind(workloadmeta.KindKubernetesKueueWorkload)
	}
	if kaiEnabled {
		c.kai = newKAIEvents(newEventIndex(kaiEventIndexCacheKey), c.startedAt, submit, listEvictEventsFromAPIServer)
		filter = filter.AddKind(workloadmeta.KindKubernetesKAIPodGroup)
	}

	c.updates = c.wmeta.Subscribe(CheckName, workloadmeta.NormalPriority, filter.Build())
	go c.receive(c.updates)
	return nil
}

// receive queues the events of each bundle. It ends when the check unsubscribes.
func (c *Check) receive(updates chan workloadmeta.EventBundle) {
	for bundle := range updates {
		bundle.Acknowledge()

		c.mu.Lock()
		c.queue = append(c.queue, bundle.Events...)
		c.mu.Unlock()
	}
}

// Cancel stops the subscription.
func (c *Check) Cancel() {
	if c.updates != nil {
		c.wmeta.Unsubscribe(c.updates)
	}
	c.CheckBase.Cancel()
}

// Run handles the updates received since the last run.
func (c *Check) Run() error {
	s, err := c.GetSender()
	if err != nil {
		return err
	}
	defer s.Commit()
	c.sender = s

	emit, leaderErr := c.leader()
	if leaderErr != nil {
		log.Warnf("Cannot tell if this Cluster Agent is the leader, scheduler events wait for the next run: %v", leaderErr)
	}

	if leaderErr != nil {
		// Keep the updates for the next run. Handling them now would record
		// their transitions as seen, and no event would be sent for them once
		// the leader is known again.
		return leaderErr
	}

	c.mu.Lock()
	updates := c.queue
	c.queue = nil
	c.mu.Unlock()

	for _, update := range updates {
		c.handle(update, emit)
	}
	if c.kai != nil {
		c.kai.endBatch()
	}

	c.pruneOnce()
	if c.kueue != nil {
		c.kueue.index.persist()
	}
	if c.kai != nil {
		c.kai.index.persist()
	}
	return leaderErr
}

func (c *Check) handle(update workloadmeta.Event, emit bool) {
	switch entity := update.Entity.(type) {
	case *workloadmeta.KubernetesKueueWorkload:
		if c.kueue == nil {
			return
		}
		switch update.Type {
		case workloadmeta.EventTypeSet:
			c.kueue.set(entity, emit)
		case workloadmeta.EventTypeUnset:
			c.kueue.unset(entity.UID)
		}
	case *workloadmeta.KubernetesKAIPodGroup:
		if c.kai == nil {
			return
		}
		switch update.Type {
		case workloadmeta.EventTypeSet:
			c.kai.set(entity, emit)
		case workloadmeta.EventTypeUnset:
			c.kai.unset(entity.UID)
		}
	}
}

// pruneOnce drops the event counts of the Workloads and PodGroups that were
// deleted while the Cluster Agent was not running.
func (c *Check) pruneOnce() {
	if c.pruned || time.Since(c.startedAt) < pruneAfter {
		return
	}
	c.pruned = true

	if c.kueue != nil {
		c.kueue.index.retain(func(uid string) bool { _, found := c.kueue.states[uid]; return found })
	}
	if c.kai != nil {
		c.kai.index.retain(func(uid string) bool { _, found := c.kai.states[uid]; return found })
	}
}

func (c *Check) send(e event.Event) {
	c.sender.Event(e)
	emittedEvents.Inc(e.SourceTypeName, transitionOf(e))
}

// transitionOf returns the transition an event was sent for, such as admitted.
func transitionOf(e event.Event) string {
	return e.EventType[strings.LastIndexByte(e.EventType, '.')+1:]
}

// isLeader tells whether this replica of the Cluster Agent is the leader. When
// leader election is off nobody is the leader and no event is sent; Configure
// warns about it once.
func isLeader(cfg config.Component) (bool, error) {
	if !cfg.GetBool("leader_election") {
		return false, nil
	}
	_, err := cluster.RunLeaderElection()
	if errors.Is(err, apiserver.ErrNotLeader) {
		return false, nil
	}
	return err == nil, err
}
