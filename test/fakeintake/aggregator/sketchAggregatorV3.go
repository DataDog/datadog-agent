// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"fmt"
	"math"
	"time"

	metricspb "github.com/DataDog/agent-payload/v5/gogen"
	intake_v3 "github.com/DataDog/agent-payload/v5/metrics/intake_v3"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// Histogram is an OTel histogram series received on /api/intake/metrics/v3/sketches.
// Explicit is set for explicit bucket histograms, Exponential for exponential ones.
type Histogram struct {
	Metric      string                              `json:"metric"`
	Tags        []string                            `json:"tags"`
	Resources   []*metricspb.MetricPayload_Resource `json:"resources"`
	Explicit    []*ExplicitHistogramPoint           `json:"explicit,omitempty"`
	Exponential []*ExponentialHistogramPoint        `json:"exponential,omitempty"`

	collectedTime time.Time
}

func (h *Histogram) name() string {
	return h.Metric
}

// GetTags returns the tags of the series.
func (h *Histogram) GetTags() []string {
	return h.Tags
}

// GetCollectedTime returns when fakeintake received the payload.
func (h *Histogram) GetCollectedTime() time.Time {
	return h.collectedTime
}

// SketchPayloadV3 is the content of a /api/intake/metrics/v3/sketches payload.
type SketchPayloadV3 struct {
	Sketches   []*Sketch    `json:"sketches"`
	Histograms []*Histogram `json:"histograms"`
}

// ParseSketchPayloadV3 decodes a /api/intake/metrics/v3/sketches payload (compressed
// protobuf). DDSketch series become Sketch, the same type used for /api/beta/sketches,
// and OTel histogram series become Histogram.
func ParseSketchPayloadV3(payload api.Payload) (*SketchPayloadV3, error) {
	out := &SketchPayloadV3{}
	if len(payload.Data) == 0 || bytes.Equal(payload.Data, []byte("{}")) {
		return out, nil
	}
	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, fmt.Errorf("v3 payload inflate: %w", err)
	}
	if len(inflated) == 0 || bytes.Equal(inflated, []byte("{}")) {
		return out, nil
	}

	var p intake_v3.Payload
	if err := proto.Unmarshal(inflated, &p); err != nil {
		return nil, fmt.Errorf("v3 payload unmarshal: %w", err)
	}
	if p.MetricData == nil {
		return out, nil
	}
	r := NewReader(p.MetricData)
	if err := r.Initialize(); err != nil {
		return nil, fmt.Errorf("v3 reader init: %w", err)
	}

	for r.HaveMoreMetrics() {
		if err := r.NextMetric(); err != nil {
			return nil, fmt.Errorf("v3 next metric: %w", err)
		}
		switch r.Type() {
		case intake_v3.MetricType_DDSketch:
			s, err := readSketchV3(r)
			if err != nil {
				return nil, err
			}
			s.collectedTime = payload.Timestamp
			out.Sketches = append(out.Sketches, s)
		case intake_v3.MetricType_ExplicitHistogram, intake_v3.MetricType_ExponentialHistogram:
			h, err := readHistogramV3(r)
			if err != nil {
				return nil, err
			}
			h.collectedTime = payload.Timestamp
			out.Histograms = append(out.Histograms, h)
		default:
			return nil, fmt.Errorf("unexpected metric type %d for %q in V3 sketches payload", r.Type(), r.Name())
		}
	}
	return out, nil
}

func resourcesV3(r *metricReaderV3) []*metricspb.MetricPayload_Resource {
	var out []*metricspb.MetricPayload_Resource
	for _, res := range r.Resources() {
		if res != nil {
			out = append(out, &metricspb.MetricPayload_Resource{Type: res[0], Name: res[1]})
		}
	}
	return out
}

func readSketchV3(r *metricReaderV3) (*Sketch, error) {
	s := &Sketch{SketchPayload_Sketch: metricspb.SketchPayload_Sketch{
		Metric: r.Name(),
		Tags:   append([]string{}, r.Tags()...),
	}}
	for _, res := range r.Resources() {
		if res != nil && res[0] == "host" {
			s.Host = res[1]
		}
	}
	for r.HaveMorePoints() {
		if err := r.NextPoint(); err != nil {
			return nil, fmt.Errorf("v3 next point: %w", err)
		}
		sum, minV, maxV, cnt := r.SketchSummary()
		k, n := r.SketchCols()
		d := metricspb.SketchPayload_Sketch_Dogsketch{
			Ts:  r.Timestamp(),
			Cnt: int64(cnt),
			Min: minV,
			Max: maxV,
			Sum: sum,
			K:   k,
			N:   make([]uint32, len(n)),
		}
		// Sketch reuses the /api/beta/sketches type, whose bin counts are uint32.
		for i, c := range n {
			if c > math.MaxUint32 {
				return nil, fmt.Errorf("sketch %q: bin count %d overflows uint32", s.Metric, c)
			}
			d.N[i] = uint32(c)
		}
		// V3 does not carry the average.
		if cnt > 0 {
			d.Avg = sum / float64(cnt)
		}
		s.Dogsketches = append(s.Dogsketches, d)
	}
	return s, nil
}

func readHistogramV3(r *metricReaderV3) (*Histogram, error) {
	h := &Histogram{
		Metric:    r.Name(),
		Tags:      append([]string{}, r.Tags()...),
		Resources: resourcesV3(r),
	}
	for r.HaveMorePoints() {
		if err := r.NextPoint(); err != nil {
			return nil, fmt.Errorf("v3 next point: %w", err)
		}
		if r.Type() == intake_v3.MetricType_ExplicitHistogram {
			p := r.ExplicitHistogram()
			h.Explicit = append(h.Explicit, &p)
		} else {
			p := r.ExponentialHistogram()
			h.Exponential = append(h.Exponential, &p)
		}
	}
	return h, nil
}

// ParseSketchesV3 returns the DDSketch series of a /api/intake/metrics/v3/sketches payload.
func ParseSketchesV3(payload api.Payload) ([]*Sketch, error) {
	p, err := ParseSketchPayloadV3(payload)
	if err != nil {
		return nil, err
	}
	return p.Sketches, nil
}

// ParseHistograms returns the OTel histogram series of a /api/intake/metrics/v3/sketches payload.
func ParseHistograms(payload api.Payload) ([]*Histogram, error) {
	p, err := ParseSketchPayloadV3(payload)
	if err != nil {
		return nil, err
	}
	return p.Histograms, nil
}

// NewSketchAggregatorV3 returns a SketchAggregator for /api/intake/metrics/v3/sketches.
// Its results have the same type as NewSketchAggregator's and can be merged with them.
func NewSketchAggregatorV3() SketchAggregator {
	return SketchAggregator{
		Aggregator: newAggregator(ParseSketchesV3),
	}
}

// HistogramAggregator stores OTel histogram series received on /api/intake/metrics/v3/sketches.
type HistogramAggregator struct {
	Aggregator[*Histogram]
}

// NewHistogramAggregator returns a new HistogramAggregator.
func NewHistogramAggregator() HistogramAggregator {
	return HistogramAggregator{
		Aggregator: newAggregator(ParseHistograms),
	}
}
