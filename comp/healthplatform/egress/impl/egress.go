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
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
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
// On each tick it calls store.GetAllIssues(), builds a HealthReport, and
// forwards it via forwarder.Send. When health_platform is disabled, lifecycle
// hooks are never registered so the goroutine is never launched.
type egress struct {
	log         log.Component
	interval    time.Duration
	hostname    string
	agentFlavor string
	store       storedef.Component
	forwarder   forwarderdef.Component

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
// When health_platform.enabled is false, it returns a zero-value struct without
// registering any lifecycle hooks.
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
		store:       reqs.Store,
		forwarder:   reqs.Forwarder,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}

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
	count, issues := e.store.GetAllIssues()
	if count == 0 {
		e.log.Debug("Health platform egress: no issues to report, skipping tick")
		return
	}

	report := e.buildReport(issues)

	// Fresh context with a fixed timeout per send: the run loop does not carry
	// a cancellable context, so we derive from Background rather than from an
	// ancestor that may have already expired or been cancelled.
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	if err := e.forwarder.Send(ctx, report); err != nil {
		e.log.Warn(fmt.Sprintf("Health platform egress: failed to send %d issues: %v", count, err))
		return
	}

	e.log.Info(fmt.Sprintf("Health platform egress: sent report with %d issues", count))
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
