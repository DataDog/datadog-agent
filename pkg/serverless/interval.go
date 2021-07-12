// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const maxInvocationsStored = 10

// StoreInvocationTime stores the given invocation time in the list of previous
// invocations. It is used to compute the invocation interval of the current function.
// It is automatically removing entries when too much have been already stored (more than maxInvocationsStored).
// When trying to store a new point, if it is older than the last one stored, it is ignored.
// Returns if the point has been stored.
func (d *Daemon) StoreInvocationTime(t time.Time) bool {
	// ignore points older than the last stored one
	if len(d.lastInvocations) > 0 && d.lastInvocations[len(d.lastInvocations)-1].After(t) {
		return false
	}

	// remove when too much/old entries
	d.lastInvocations = append(d.lastInvocations, t)
	if len(d.lastInvocations) > maxInvocationsStored {
		d.lastInvocations = d.lastInvocations[len(d.lastInvocations)-maxInvocationsStored : len(d.lastInvocations)]
	}

	return true
}

// InvocationInterval computes the invocation interval of the current function.
// This function returns 0 if not enough invocations were done.
func (d *Daemon) InvocationInterval() time.Duration {
	// with less than 3 invocations, we don't have enough data to compute
	// something reliable.
	if len(d.lastInvocations) < 3 {
		return 0
	}

	var total int64
	for i := 1; i < len(d.lastInvocations); i++ {
		total += int64(d.lastInvocations[i].Sub(d.lastInvocations[i-1]))
	}

	return time.Duration(total / int64(len(d.lastInvocations)-1))
}

// AutoSelectStrategy uses the invocation interval of the function to select the
// best flush strategy.
// This function doesn't mind if the flush strategy has been overridden through
// configuration / environment var, the caller is responsible of that.
func (d *Daemon) AutoSelectStrategy() flush.Strategy {
	flushInterval := 10 * time.Second
	freq := d.InvocationInterval()

	if !d.clientLibReady {
		return flush.NewPeriodically(flushInterval)
	}

	// when not enough data is available, fallback on flush.AtTheEnd strategy
	if freq == time.Duration(0) {
		return &flush.AtTheEnd{}
	}

	// if running more than 1 time every 5 minutes, we can switch to the flush strategy
	// flushing at least every 10 seconds (at the start of the invocation)
	// TODO(remy): compute a proper interval instead of hard-coding 10 seconds
	if freq.Seconds() < 60*5 {
		return flush.NewPeriodically(flushInterval)
	}

	return &flush.AtTheEnd{}
}

// UpdateStrategy will update the current flushing strategy
func (d *Daemon) UpdateStrategy() {
	if d.useAdaptiveFlush {
		newStrat := d.AutoSelectStrategy()
		if newStrat.String() != d.flushStrategy.String() {
			log.Debug("Switching to flush strategy:", newStrat)
			d.flushStrategy = newStrat
		}
	}
}
