// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/atomic"
)

// Orchestrator defines the contract for the extension
type Orchestrator interface {
	HitHelloRoute()
	HitFlushRoute()
	HitTimeout()
	HitEndInvocation()
	WaitForExtensionLifecycle()
	RuntimeDoneReceived()
	Reset()
	IsFlushPossible() bool
	CanAcceptTraces()
	HitInvokeEvent()
}

type lambdaOrchestrator struct {
	hasHitHelloRoute       *atomic.Bool
	hasHitFlushRoute       *atomic.Bool
	hasHitEndInvocation    *atomic.Bool
	hasHitTimeout          *atomic.Bool
	hasReceivedRuntimeDone *atomic.Bool
	hasInvocationStarted   *atomic.Bool
}

// NewLambdaOrchestrator creates a new LambdaOrchestrator
func NewLambdaOrchestrator() Orchestrator {
	orch := &lambdaOrchestrator{}
	orch.Reset()
	return orch
}

// HitHelloRoute tells the orchestrator that hello route has been hit
func (lo *lambdaOrchestrator) HitHelloRoute() {
	log.Debug("[orchestrator] hit hello route")
	lo.hasHitHelloRoute.Store(true)
}

// HitFlushRoute tells the orchestrator that flush route has been hit
func (lo *lambdaOrchestrator) HitFlushRoute() {
	log.Debug("[orchestrator] hit flush route")
	lo.hasHitFlushRoute.Store(true)
}

// HitEndInvocation tells the orchestrator that end-invocation has been hit
func (lo *lambdaOrchestrator) HitEndInvocation() {
	log.Debug("[orchestrator] hit end invocation")
	lo.hasHitEndInvocation.Store(true)
}

// HitTimeout tells the orchestrator that the timeout has been reached
func (lo *lambdaOrchestrator) HitTimeout() {
	log.Debug("[orchestrator] hit timeout")
	lo.hasHitTimeout.Store(true)
}

// HitTimeout tells the orchestrator that a RuntimeDone log has been received
func (lo *lambdaOrchestrator) RuntimeDoneReceived() {
	log.Debug("[orchestrator] RuntimeDoneReceived")
	lo.hasReceivedRuntimeDone.Store(true)
}

// HitInvokeEvent tells the orchestrator that a Invoke envet has been received
func (lo *lambdaOrchestrator) HitInvokeEvent() {
	log.Debug("[orchestrator] HitInvokeEvent")
	lo.hasInvocationStarted.Store(true)
}

// Reset sets the Orchestrator to a clean state after each invocation
func (lo *lambdaOrchestrator) Reset() {
	log.Debug("[orchestrator] Reset")
	lo.hasHitHelloRoute = atomic.NewBool(false)
	lo.hasHitFlushRoute = atomic.NewBool(false)
	lo.hasHitEndInvocation = atomic.NewBool(false)
	lo.hasHitTimeout = atomic.NewBool(false)
	lo.hasReceivedRuntimeDone = atomic.NewBool(false)
	lo.hasInvocationStarted = atomic.NewBool(false)
}

// WaitForExtensionLifecycle is blocking
// It waits until all work in done before telling the runtime API
// that a new invocation could be handle
func (lo *lambdaOrchestrator) WaitForExtensionLifecycle() {
	log.Debug("[orchestrator] WaitForExtensionLifecycle blocked")
	for {
		if lo.hasHitTimeout.Load() {
			log.Debug("[orchestrator] WaitForExtensionLifecycle unblocked (timeout)")
			return
		}
		if lo.hasHitFlushRoute.Load() || lo.hasHitEndInvocation.Load() {
			log.Debug("[orchestrator] WaitForExtensionLifecycle unblocked")
			lo.Reset()
			return
		}
	}
}

// IsFlushPossible tells whether or not a flush can be performed
func (lo *lambdaOrchestrator) IsFlushPossible() bool {
	return lo.hasReceivedRuntimeDone.Load()
}

// CanAcceptTraces is blocking. Wait for an Invoke event
func (lo *lambdaOrchestrator) CanAcceptTraces() {
	log.Debug("[orchestrator] CanAcceptTraces blocked")
	for {
		if lo.hasInvocationStarted.Load() {
			log.Debug("[orchestrator] CanAcceptTraces unblocked")
			return
		}
	}
}
