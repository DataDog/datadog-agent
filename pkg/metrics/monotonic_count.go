// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metrics

// MonotonicCount tracks a raw counter, based on increasing counter values.
// Samples that have a lower value than the previous sample are ignored (since it usually
// means that the underlying raw counter has been reset).
// Example:
//  submitting samples 2, 3, 6, 7 returns 5 (i.e. 7-2) on flush ;
//  then submitting samples 10, 11 on the same MonotonicCount returns 4 (i.e. 11-7) on flush
type MonotonicCount struct {
	previousSample        float64
	currentSample         float64
	sampledSinceLastFlush bool
	hasPreviousSample     bool
	value                 float64
}

func (mc *MonotonicCount) addSample(sample MetricSampleValue, timestamp float64) {
	if !mc.sampledSinceLastFlush {
		mc.currentSample = sample.Value
		mc.sampledSinceLastFlush = true
	} else {
		mc.previousSample, mc.currentSample = mc.currentSample, sample.Value
		mc.hasPreviousSample = true
	}

	// To handle cases where the samples are not monotonically increasing, we always add the difference
	// between 2 consecutive samples to the value that'll be flushed (if the difference is >0).
	diff := mc.currentSample - mc.previousSample
	if mc.sampledSinceLastFlush && mc.hasPreviousSample && diff > 0. {
		mc.value += diff
	}
}

func (mc *MonotonicCount) flush(timestamp float64) ([]*Serie, error) {
	if !mc.sampledSinceLastFlush || !mc.hasPreviousSample {
		return []*Serie{}, NoSerieError{}
	}

	value := mc.value
	// reset struct fields
	mc.previousSample, mc.currentSample, mc.value = mc.currentSample, 0., 0.
	mc.sampledSinceLastFlush = false

	return []*Serie{
		{
			// we use the timestamp passed to the flush
			Points: []Point{{Ts: timestamp, Value: value}},
			MType:  APICountType,
		},
	}, nil
}
