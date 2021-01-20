package serverless

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
)

const maxInvocationsStored = 50

// StoreInvocationTime stores the given invocation time in the list of previous
// invocations. It is used to compute the invocation frequency of the current function.
// It is automatically removing entries when too much have been already stored (more than maxInvocationsStored).
// When trying to store a new point, if it is older than the last one stored, it is ignored.
// Returns if the point has been stored.
func (d *Daemon) StoreInvocationTime(t time.Time) bool {
	// ignore points less recent than the last stored one
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

// InvocationFrequency computes the invocation frequency of the current function.
// This function returns 0 if not enough invocations were done.
func (d *Daemon) InvocationFrequency() time.Duration {
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

// AutoSelectStrategy uses the invocation frequency of the function to select the
// best flush strategy.
// This function doesn't mind if the flush strategy has been overriden through
// configuration / environment var, the caller is responsible of that.
func (d *Daemon) AutoSelectStrategy() flush.Strategy {
	freq := d.InvocationFrequency()

	// when not enough data's available, fallback on flush.AtTheEnd strategy
	if freq == time.Duration(0) {
		return &flush.AtTheEnd{}
	}

	// if the function is running more than 1 time a minute, we can switch to the flush strategy
	// flushing at least every 10 seconds.
	if freq.Seconds() <= 60 {
		return &flush.AtLeast{N: 10 * time.Second}
	}

	// if running more than 1 time every 5 minutes, we can switch to a "flush at the start
	// of the function" strategy.
	if freq.Seconds() < 60*5 {
		return &flush.AtTheStart{}
	}

	return &flush.AtTheEnd{}
}
