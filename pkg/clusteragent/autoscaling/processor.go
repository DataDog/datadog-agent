// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"context"
	"time"
)

// ProcessResult defines the queuing policy after processing the object
type ProcessResult struct {
	// Requeue tells the Controller to requeue the reconcile key. Defaults to false.
	Requeue bool

	// RequeueAfter if greater than 0, tells the Controller to requeue the reconcile key after the Duration.
	// Implies that Requeue is true, there is no need to set Requeue to true at the same time as RequeueAfter.
	RequeueAfter time.Duration
}

// ShouldRequeue is small helper to know if we should requeue
func (p ProcessResult) ShouldRequeue() bool {
	return p.Requeue || p.RequeueAfter > 0
}

// After returns a copy of current ProcessResult with RequeueAfter set to the given duration
func (p ProcessResult) After(after time.Duration) ProcessResult {
	p.RequeueAfter = after
	return p
}

// Merge merges the other ProcessResult into a newly returned one
func (p ProcessResult) Merge(other ProcessResult) ProcessResult {
	if other.Requeue {
		p.Requeue = true

		if p.RequeueAfter > 0 {
			p.RequeueAfter = min(p.RequeueAfter, other.RequeueAfter)
		} else {
			p.RequeueAfter = other.RequeueAfter
		}
	}

	return p
}

var (
	// Requeue is a shortcut to avoid having ProcessResult{Requeue: true} everywhere in the code
	Requeue = ProcessResult{Requeue: true}

	// NoRequeue is a shortcut to avoid having ProcessResult{} everywhere in the code
	NoRequeue = ProcessResult{}
)

// Processor defines the interface that needs to be implemented by the controller
type Processor interface {
	// Process is called by the controller to process an object
	Process(ctx context.Context, key, ns, name string) ProcessResult
}

// ProcessorPreStart is an interface that can be implemented by the Processor to perform some initialization after informers are synced and before the controller starts
type ProcessorPreStart interface {
	// PreStart is called by the controller before starting workers
	PreStart(ctx context.Context)
}
