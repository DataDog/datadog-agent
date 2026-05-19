// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package batchsize

// NewOptimizerForTest constructs an Optimizer with explicit internal state for
// use in tests in other packages. Not for production use.
func NewOptimizerForTest(configBatchSize, batchSize int, failures map[int]int, lastSuccessfulBatchSize int) *Optimizer {
	if failures == nil {
		failures = make(map[int]int)
	}
	return &Optimizer{
		configBatchSize:         configBatchSize,
		batchSize:               batchSize,
		failuresByBatchSize:     failures,
		lastSuccessfulBatchSize: lastSuccessfulBatchSize,
	}
}

// FailuresByBatchSize exposes the internal failure map for assertions in tests.
func (o *Optimizer) FailuresByBatchSize() map[int]int {
	return o.failuresByBatchSize
}

// LastSuccessfulBatchSize exposes the internal last-successful tracker for assertions in tests.
func (o *Optimizer) LastSuccessfulBatchSize() int {
	return o.lastSuccessfulBatchSize
}

// SetFailuresByBatchSize replaces the internal failure map. For tests only.
func (o *Optimizer) SetFailuresByBatchSize(failures map[int]int) {
	o.failuresByBatchSize = failures
}
