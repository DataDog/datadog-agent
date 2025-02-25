// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
)

// Rate tracks the rate of a metric over 2 successive flushes
type Rate struct {
	previousSample    float64
	previousTimestamp float64
	sample            float64
	timestamp         float64
}

func (r *Rate) addSample(sample *MetricSample, timestamp float64) {
	if r.timestamp != 0 {
		r.previousSample, r.previousTimestamp = r.sample, r.timestamp
	}
	r.sample, r.timestamp = sample.Value, timestamp

}

//nolint:revive // TODO(AML) Fix revive linter
func (r *Rate) flush(_ float64) ([]SerieData, error) {
	if r.previousTimestamp == 0 || r.timestamp == 0 {
		return nil, NoSerieError{}
	}

	if r.timestamp == r.previousTimestamp {
		return nil, fmt.Errorf("Rate was sampled twice at the same timestamp, can't compute a rate")
	}

	value, ts := (r.sample-r.previousSample)/(r.timestamp-r.previousTimestamp), r.timestamp
	r.previousSample, r.previousTimestamp = r.sample, r.timestamp
	r.sample, r.timestamp = 0., 0.

	if value < 0 {
		return nil, fmt.Errorf("Rate value is negative, discarding it (the underlying counter may have been reset)")
	}

	return []SerieData{
		{
			Point: Point{Ts: ts, Value: value},
			MType: APIGaugeType,
		},
	}, nil
}

func (r *Rate) isStateful() bool {
	return true
}
