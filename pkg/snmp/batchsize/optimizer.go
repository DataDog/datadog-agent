// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package batchsize provides an adaptive feedback controller for an integer
// "batch size" parameter. Callers report success or failure of each operation
// and the controller halves on failure (with a floor at the last successful
// value) and increments back toward the configured ceiling on success.
//
// The primitive is generic: callers decide what the integer represents
// (number of OIDs per request, max-repetitions, etc.) and what counts as a
// success or failure.
package batchsize

import "github.com/DataDog/datadog-agent/pkg/util/log"

const (
	onSuccessIncreaseValue  = 1
	onFailureDecreaseFactor = 2

	maxFailuresPerWindow = 2
)

// Optimizer is a feedback controller on a positive integer batch size.
type Optimizer struct {
	name                    string
	configBatchSize         int
	batchSize               int
	failuresByBatchSize     map[int]int
	lastSuccessfulBatchSize int
}

// NewOptimizer returns an Optimizer starting at configBatchSize. The name is
// used only to label the debug logs the optimizer emits when it adjusts the
// batch size.
func NewOptimizer(configBatchSize int, name string) *Optimizer {
	return &Optimizer{
		name:                name,
		configBatchSize:     configBatchSize,
		batchSize:           configBatchSize,
		failuresByBatchSize: make(map[int]int),
	}
}

// BatchSize returns the current batch size.
func (o *Optimizer) BatchSize() int {
	return o.batchSize
}

// OnFailure halves the batch size (with a floor at 1, never crossing below
// the last successful value) and returns whether the batch size changed.
// A true return means a retry at the new size is worth attempting.
func (o *Optimizer) OnFailure() bool {
	o.failuresByBatchSize[o.batchSize]++

	oldBatchSize := o.batchSize

	newBatchSize := max(o.batchSize/onFailureDecreaseFactor, 1)
	if oldBatchSize > o.lastSuccessfulBatchSize && newBatchSize < o.lastSuccessfulBatchSize {
		newBatchSize = o.lastSuccessfulBatchSize
	}

	o.batchSize = newBatchSize

	log.Debugf("%s with batch size %d failed, new batch size is %d", o.name, oldBatchSize, newBatchSize)

	return oldBatchSize != newBatchSize
}

// OnSuccess increments the batch size toward the configured ceiling, skipping
// sizes that have already failed too many times in the current failure window.
func (o *Optimizer) OnSuccess() {
	o.lastSuccessfulBatchSize = o.batchSize

	if o.batchSize >= o.configBatchSize {
		return
	}

	newBatchSize := min(o.batchSize+onSuccessIncreaseValue, o.configBatchSize)
	if o.failuresByBatchSize[newBatchSize] >= maxFailuresPerWindow {
		return
	}

	log.Debugf("%s with batch size %d succeeded, new batch size is %d", o.name, o.batchSize, newBatchSize)

	o.batchSize = newBatchSize
}

// RefreshFailures clears the failure-count map. Callers managing a failure
// window should call this when the window expires.
func (o *Optimizer) RefreshFailures() {
	o.failuresByBatchSize = make(map[int]int)
}
