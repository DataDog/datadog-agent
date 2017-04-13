package aggregator

import (
	"fmt"
)

// Rate tracks the rate of a metric over 2 successive flushes
type Rate struct {
	previousSample    float64
	previousTimestamp int64
	sample            float64
	timestamp         int64
}

func (r *Rate) addSample(sample *MetricSample, timestamp int64) {
	if r.timestamp != 0 {
		r.previousSample, r.previousTimestamp = r.sample, r.timestamp
	}
	r.sample, r.timestamp = sample.Value, timestamp
}

func (r *Rate) flush(timestamp int64) ([]*Serie, error) {
	if r.previousTimestamp == 0 || r.timestamp == 0 {
		return []*Serie{}, NoSerieError{}
	}

	if r.timestamp == r.previousTimestamp {
		return []*Serie{}, fmt.Errorf("Rate was sampled twice at the same timestamp, can't compute a rate")
	}

	value, ts := (r.sample-r.previousSample)/float64(r.timestamp-r.previousTimestamp), r.timestamp
	r.previousSample, r.previousTimestamp = r.sample, r.timestamp
	r.sample, r.timestamp = 0, 0

	return []*Serie{
		&Serie{
			Points: []Point{{Ts: ts, Value: value}},
			MType:  APIGaugeType,
		},
	}, nil
}
