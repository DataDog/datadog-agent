// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package healthcheck

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

// ServiceCheckObserver is a non-blocking handle installed before check senders are created.
type ServiceCheckObserver interface {
	ObserveServiceCheck(id checkid.ID, scName string, status servicecheck.ServiceCheckStatus)
}

// RemediationDispatcher handles requests on the observer worker and must honor context cancellation.
type RemediationDispatcher interface {
	Dispatch(ctx context.Context, id checkid.ID, scName string, cfg *integration.HealthCheckConfig)
}

type observation struct {
	id         checkid.ID
	name       string
	status     servicecheck.ServiceCheckStatus
	generation uint64
	at         time.Time
}

// Observer hands service checks to a bounded worker without blocking their submission.
type Observer struct {
	input   chan observation
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	dropped atomic.Uint64
}

// NewObserver starts an ordered worker for asynchronous remediation dispatch.
func NewObserver(dispatcher RemediationDispatcher) *Observer {
	if dispatcher == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	o := &Observer{input: make(chan observation, 128), ctx: ctx, cancel: cancel, done: make(chan struct{})}
	go o.run(dispatcher)
	return o
}

// ObserveServiceCheck drops observations if the queue is full.
func (o *Observer) ObserveServiceCheck(id checkid.ID, scName string, status servicecheck.ServiceCheckStatus) {
	if o == nil || id == "" || o.ctx.Err() != nil {
		return
	}
	select {
	case o.input <- observation{id: id, name: scName, status: status, generation: registrationGeneration.Load(), at: time.Now()}:
	default:
		o.dropped.Add(1)
	}
}

// Dropped returns the number of observations discarded due to backpressure.
func (o *Observer) Dropped() uint64 {
	if o == nil {
		return 0
	}
	return o.dropped.Load()
}

// Stop cancels outstanding dispatch and waits for the worker to exit.
func (o *Observer) Stop() {
	if o == nil {
		return
	}
	o.cancel()
	<-o.done
}

func (o *Observer) run(dispatcher RemediationDispatcher) {
	defer close(o.done)
	for {
		select {
		case <-o.ctx.Done():
			return
		case obs := <-o.input:
			if o.ctx.Err() != nil {
				return
			}
			if cfg := transition(obs); cfg != nil {
				dispatcher.Dispatch(o.ctx, obs.id, obs.name, cfg)
			}
		}
	}
}

func transition(obs observation) *integration.HealthCheckConfig {
	registry.Lock()
	defer registry.Unlock()
	entry := registry.checks[obs.id]
	if entry == nil || entry.generation > obs.generation || (entry.config.ServiceCheck != "" && entry.config.ServiceCheck != obs.name) {
		return nil
	}
	previous, seen := entry.statuses[obs.name]
	entry.statuses[obs.name] = obs.status
	if !seen || previous != servicecheck.ServiceCheckOK || obs.status != servicecheck.ServiceCheckCritical {
		return nil
	}
	// Attempts share a cooldown window across service-check names for this check ID.
	if entry.windowStart.IsZero() || obs.at.Sub(entry.windowStart) >= entry.cooldown {
		entry.windowStart = obs.at
		entry.attempts = 0
	}
	if entry.attempts >= entry.maxAttempts {
		return nil
	}
	entry.attempts++
	return cloneConfig(entry.config)
}
