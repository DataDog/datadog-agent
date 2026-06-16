// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package egressimpl implements the health platform egress component.
package egressimpl

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	egressdef "github.com/DataDog/datadog-agent/comp/healthplatform/egress/def"
	forwarderdef "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	defaultEgressInterval = 15 * time.Minute
	sendTimeout           = 30 * time.Second
	eventType             = "agent-health-issues"

	issueChSize = 2048
)

// egress drives the periodic outbound POST to the Datadog intake.
type egress struct {
	log         log.Component
	interval    time.Duration
	hostname    string
	agentFlavor string
	forwarder   forwarderdef.Component

	activeCh   chan *healthplatform.Issue // new/ongoing issues
	resolvedCh chan *healthplatform.Issue // resolved issues; flushed after successful send

	stopCh chan struct{}
	doneCh chan struct{}
}

// Requires defines the dependencies for the egress component.
type Requires struct {
	Lifecycle compdef.Lifecycle
	Log       log.Component
	Config    config.Component
	Hostname  hostnameinterface.Component
	Store     storedef.Component
	Forwarder forwarderdef.Component
}

// New creates the egress component and registers its lifecycle hooks.
func New(reqs Requires) egressdef.Component {
	if !reqs.Config.GetBool("health_platform.enabled") {
		return &egress{}
	}

	hostname, err := reqs.Hostname.Get(context.Background())
	if err != nil {
		reqs.Log.Warn("Health platform egress: failed to get hostname: " + err.Error())
		hostname = ""
	}

	interval := reqs.Config.GetDuration("health_platform.forwarder.interval")
	if interval <= 0 {
		interval = defaultEgressInterval
	}

	e := &egress{
		log:         reqs.Log,
		interval:    interval,
		hostname:    hostname,
		agentFlavor: flavor.GetFlavor(),
		forwarder:   reqs.Forwarder,
		activeCh:    make(chan *healthplatform.Issue, issueChSize),
		resolvedCh:  make(chan *healthplatform.Issue, issueChSize),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}

	// Register before OnStart so loadFromDisk can pre-populate resolvedCh.
	reqs.Store.RegisterIssuesObserver(storedef.IssuesObserver{
		ActiveCh:   e.activeCh,
		ResolvedCh: e.resolvedCh,
	})

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: e.start,
		OnStop:  e.stop,
	})

	return e
}

func (e *egress) start(_ context.Context) error {
	e.log.Info(fmt.Sprintf("Starting health platform egress with %v interval", e.interval))
	go e.run()
	return nil
}

func (e *egress) stop(_ context.Context) error {
	e.log.Info("Stopping health platform egress")
	close(e.stopCh)
	<-e.doneCh
	return nil
}

func (e *egress) run() {
	defer close(e.doneCh)

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.tick()
		case <-e.stopCh:
			return
		}
	}
}

func (e *egress) tick() {
	active := snapshotIssues(e.activeCh)
	resolved := snapshotIssues(e.resolvedCh)

	if len(active) == 0 && len(resolved) == 0 {
		e.log.Debug("Health platform egress: no issues to report, skipping tick")
		return
	}

	merged := make(map[string]*healthplatform.Issue, len(active)+len(resolved))
	for _, i := range active {
		merged[i.Id] = i
	}
	for _, r := range resolved { // resolved state wins over active for the same ID
		merged[r.Id] = r
	}

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	if err := e.forwarder.Send(ctx, e.buildReport(merged)); err != nil {
		e.log.Warn(fmt.Sprintf("Health platform egress: failed to send %d issues: %v", len(merged), err))
		return
	}

	e.log.Info(fmt.Sprintf("Health platform egress: sent report with %d issues", len(merged)))

	// Dequeue the tombstones we just sent; any that arrived during the send stay for next tick.
	for range len(resolved) {
		select {
		case <-e.resolvedCh:
		default:
		}
	}
}

// snapshotIssues drains up to len(ch) items then re-queues them, leaving the channel intact.
func snapshotIssues(ch chan *healthplatform.Issue) []*healthplatform.Issue {
	n := len(ch)
	if n == 0 {
		return nil
	}
	items := make([]*healthplatform.Issue, 0, n)
	for i := 0; i < n; i++ {
		select {
		case item := <-ch:
			items = append(items, item)
		default:
			n = i // channel drained early; stop without iterating further
		}
	}
	for _, item := range items {
		select {
		case ch <- item:
		default:
		}
	}
	return items
}

func (e *egress) buildReport(issues map[string]*healthplatform.Issue) *healthplatform.HealthReport {
	return &healthplatform.HealthReport{
		EventType: eventType,
		EmittedAt: time.Now().UTC().Format(time.RFC3339),
		Service:   e.agentFlavor,
		Host: &healthplatform.HostInfo{
			Hostname:     e.hostname,
			AgentVersion: pointer.Ptr(version.AgentVersion),
		},
		Issues: issues,
	}
}
