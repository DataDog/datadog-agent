// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

// MonotonicCount tracks a raw counter, based on increasing counter values.
// Samples that have a lower value than the previous sample are ignored (since it usually
// means that the underlying raw counter has been reset).
// Example:
//
//	submitting samples 2, 3, 6, 7 returns 5 (i.e. 7-2) on flush ;
//	then submitting samples 10, 11 on the same MonotonicCount returns 4 (i.e. 11-7) on flush
type MonotonicCount struct {
	previousSample        float64
	currentSample         float64
	sampledSinceLastFlush bool
	hasPreviousSample     bool
	value                 float64
	// With flushFirstValue enabled (passed in MetricSample), these 2 differences apply:
	// 1. the sampled value will be flushed as-is if it's the first value sampled (and no other
	//    values are flushed until the flush). The assumption is that the underlying raw counter
	//    started from 0 and that any earlier value of the raw counter would'be been sampled
	//    earlier, so it's safe to flush the raw value as-is.
	// 2. a sample that has a lower value than the previous sample is not ignored, instead its
	//    value is used as the value to flush. The assumption is that the underlying raw counter was
	//    reset from 0.
	// This flag is used (for example) by the openmetrics check after its first run, to better
	// support openmetrics monotonic counters.
	flushFirstValue bool
}

//nolint:revive // TODO(AML) Fix revive linter
func (mc *MonotonicCount) addSample(sample *MetricSample, _ float64) {
	if !mc.sampledSinceLastFlush {
		mc.currentSample = sample.Value
		mc.sampledSinceLastFlush = true
	} else {
		mc.previousSample, mc.currentSample = mc.currentSample, sample.Value
		mc.hasPreviousSample = true
	}

	mc.flushFirstValue = sample.FlushFirstValue

	// To handle cases where the samples are not monotonically increasing, we always add the difference
	// between 2 consecutive samples to the value that'll be flushed (if the difference is >0).
	diff := mc.currentSample - mc.previousSample
	if (mc.hasPreviousSample || mc.flushFirstValue) && diff >= 0. {
		mc.value += diff
	} else if mc.flushFirstValue {
		mc.value = mc.currentSample
	}
}

func (mc *MonotonicCount) flush(timestamp float64) ([]*Serie, error) {
	if !mc.sampledSinceLastFlush || !(mc.hasPreviousSample || mc.flushFirstValue) {
		return []*Serie{}, NoSerieError{}
	}

	value := mc.value
	// reset struct fields
	mc.previousSample, mc.currentSample, mc.value = mc.currentSample, 0., 0.
	mc.hasPreviousSample = true
	mc.sampledSinceLastFlush = false
	mc.flushFirstValue = false

	return []*Serie{
		{
			// we use the timestamp passed to the flush
			Points: []Point{{Ts: timestamp, Value: value}},
			MType:  APICountType,
		},
	}, nil
}

func (mc *MonotonicCount) isStateful() bool {
	return true
}
