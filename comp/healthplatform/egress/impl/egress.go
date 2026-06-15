// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package egressimpl implements the health platform egress component.
package egressimpl

import (
	"context"
	"fmt"
	"sync"
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
	// defaultEgressInterval is the default interval between health report submissions.
	// Matches the previous forwarder default for backward compatibility.
	defaultEgressInterval = 15 * time.Minute

	// sendTimeout is the HTTP timeout for a single forwarder.Send call.
	sendTimeout = 30 * time.Second

	// eventType is the health report event type consumed by the intake.
	eventType = "agent-health-issues"
)

// egress drives the periodic outbound POST to the Datadog intake.
// active issues are re-sent every tick; resolved tombstones (toSendOnce) are
// trimmed by a slice-length operation after a successful send, so exactly-once
// forwarding is structural rather than conditional.
type egress struct {
	log         log.Component
	interval    time.Duration
	hostname    string
	agentFlavor string
	forwarder   forwarderdef.Component

	pendingMu  sync.Mutex
	active     map[string]*healthplatform.Issue // re-sent every tick until resolved
	toSendOnce []*healthplatform.Issue          // tombstones; trimmed after a successful send

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
		active:      make(map[string]*healthplatform.Issue),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}

	// Callbacks must be registered before OnStart: the store's loadFromDisk fires
	// first and calls onResolveIssue for any RESOLVED issues found on disk.
	reqs.Store.SetEgressCallbacks(storedef.EgressCallbacks{
		OnReportIssue:  e.onReportIssue,
		OnResolveIssue: e.onResolveIssue,
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

func (e *egress) onReportIssue(issue *healthplatform.Issue) {
	e.pendingMu.Lock()
	e.active[issue.Id] = issue
	e.pendingMu.Unlock()
}

// onResolveIssue removes the issue from active and queues the tombstone for one send.
func (e *egress) onResolveIssue(tombstone *healthplatform.Issue) {
	e.pendingMu.Lock()
	delete(e.active, tombstone.Id)
	e.toSendOnce = append(e.toSendOnce, tombstone)
	e.pendingMu.Unlock()
}

func (e *egress) tick() {
	e.pendingMu.Lock()
	if len(e.active) == 0 && len(e.toSendOnce) == 0 {
		e.pendingMu.Unlock()
		e.log.Debug("Health platform egress: no issues to report, skipping tick")
		return
	}

	activeSnap := make(map[string]*healthplatform.Issue, len(e.active))
	for k, v := range e.active {
		activeSnap[k] = v
	}
	// Take a reference without clearing: we trim by length after a successful
	// send, so tombstones appended by onResolveIssue during the send are kept.
	resolvedSnap := e.toSendOnce
	e.pendingMu.Unlock()

	merged := make(map[string]*healthplatform.Issue, len(activeSnap)+len(resolvedSnap))
	for k, v := range activeSnap {
		merged[k] = v
	}
	for _, t := range resolvedSnap {
		merged[t.Id] = t
	}

	report := e.buildReport(merged)

	// Derive from Background: the run loop has no parent context to cancel.
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	if err := e.forwarder.Send(ctx, report); err != nil {
		e.log.Warn(fmt.Sprintf("Health platform egress: failed to send %d issues: %v", len(merged), err))
		return
	}

	e.log.Info(fmt.Sprintf("Health platform egress: sent report with %d issues", len(merged)))

	if len(resolvedSnap) > 0 {
		e.pendingMu.Lock()
		e.toSendOnce = e.toSendOnce[len(resolvedSnap):]
		e.pendingMu.Unlock()
	}
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
