// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package stats

import (
	"fmt"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// SublayerValue is just a span-metric placeholder for a given sublayer val
type SublayerValue struct {
	Metric string
	Tag    Tag
	Value  float64
}

// String returns a description of a sublayer value.
func (v SublayerValue) String() string {
	if v.Tag.Name == "" && v.Tag.Value == "" {
		return fmt.Sprintf("SublayerValue{%q, %v}", v.Metric, v.Value)
	}

	return fmt.Sprintf("SublayerValue{%q, %v, %v}", v.Metric, v.Tag, v.Value)
}

// GoString returns a description of a sublayer value.
func (v SublayerValue) GoString() string {
	return v.String()
}

type SublayerCalculator struct {
	activeSpans      []int
	activeSpansIndex []int
	openedSpans      []bool
	nChildren        []int
	execDuration     []float64
	parentIdx        []int
	timestamps       TimestampArray
	maxSpans         int
}

func NewSublayersCalculator(maxSpans int) *SublayerCalculator {
	c := &SublayerCalculator{}
	c.initFields(maxSpans)
	return c
}

func (c *SublayerCalculator) initFields(maxSpans int) {
	// add 1 because last field is reserved for spans that are not in the trace (spanID 0 for eg)
	n := maxSpans+1
	c.maxSpans = maxSpans
	c.activeSpans=      make([]int, n)
	c.activeSpansIndex= make([]int, n)
	c.openedSpans=      make([]bool, n)
	c.nChildren=        make([]int, n)
	c.execDuration =          make([]float64, n)
	c.parentIdx=        make([]int, n)
	c.timestamps=       make(TimestampArray, 2*maxSpans)
}

func (c *SublayerCalculator) reset(n int) {
	if c.maxSpans < n {
		c.initFields(n)
	}
	for i := 0; i < n+1; i++ {
		c.activeSpansIndex[i] = -1
		c.activeSpans[i] = 0
		c.openedSpans[i] = false
		c.nChildren[i] = 0
		c.execDuration[i] = 0
	}
}

type timestamp struct {
	spanStart bool
	spanIdx    int
	parentIdx  int
	ts        int64
}

type TimestampArray []timestamp

func (t TimestampArray) Len() int      { return len(t) }
func (t TimestampArray) Swap(i, j int) { t[i], t[j] = t[j], t[i] }

// for spans with a duration of 0, we need to open them before closing them
func (t TimestampArray) Less(i, j int) bool {
	return t[i].ts < t[j].ts || (t[i].ts == t[j].ts && t[i].spanStart && !t[j].spanStart)
}

func (c *SublayerCalculator) buildTimestamps(trace pb.Trace) {
	for i, span := range trace {
		c.timestamps[2*i] = timestamp{spanStart: true, spanIdx: i, parentIdx: c.parentIdx[i], ts: span.Start}
		c.timestamps[2*i+1] = timestamp{spanStart: false, spanIdx: i, parentIdx: c.parentIdx[i], ts: span.Start+span.Duration}
	}
	sort.Sort(c.timestamps[:2*len(trace)])
}

func (c *SublayerCalculator) computeParentIdx(trace pb.Trace) {
	idToIdx := make(map[uint64]int, len(trace))
	for idx, span := range trace {
		idToIdx[span.SpanID] = idx
	}
	for idx, span := range trace {
		if parentIdx, ok := idToIdx[span.ParentID]; ok {
			c.parentIdx[idx] = parentIdx
		} else {
			// unknown parent (or root)
			c.parentIdx[idx] = len(trace)
		}
	}
}


// ComputeSublayers extracts sublayer values by type and service for a trace
//
// Description of the algorithm, with the following trace as an example:
//
// 0  10  20  30  40  50  60  70  80  90 100 110 120 130 140 150
// |===|===|===|===|===|===|===|===|===|===|===|===|===|===|===|
// <-1------------------------------------------------->
//     <-2----------------->       <-3--------->
//         <-4--------->
//       <-5------------------->
//                         <--6-------------------->
//                                             <-7------------->
// idx 0 id 1: service=web-server, type=web,   parent=nil
// idx 1 id 2: service=pg,         type=db,    parent=1
// idx 2 3: service=render,     type=web,   parent=1
// idx 3 4: service=pg-read,    type=db,    parent=2
// idx 4 5: service=redis,      type=cache, parent=1
// idx 5 6: service=rpc1,       type=rpc,   parent=1
// idx 6 7: service=alert,      type=rpc,   parent=6
//
// Step 1: Find all time intervals to consider (set of start/end time
//         of spans). For each timestamp, store the spanIdx, parentIdx and if it's the open or close timestamp:
// if the parent span is not in the trace, parent idx is set to len(trace)
//
//         [(0, open, span idx 0, parent idx 7), (10, open, span idx 1, parent idx 0), ... (150, close, span idx 6, parent idx 5)]
//
// Step 2: Go through all the ordered timestamps (start and ends of each span) in the trace
// Keep track of which are the opened spans at any given timestamp
// Keep track of the active spans (opened, and with no children) at any given timestamp
// in order to know when a span has no more children, Keep track of the number of children for each span at any given timestamp
// for each period between two timestamps, it knows which were the active spans. And it increases the exec duration for them by
// (interval duration / nActiveSpans)
//
//         {
//             idx 0 (spanID 1): 10/1 (between tp 0 and 10) + 10/2 (between tp 120 and 130) = 15,
//             ...
//             idx 6 (spanID 7): 10/2 (between tp 110 and 120) + 10/2 (between tp 120 and 130) + 20/1 (between tp 130 and 150),
//         }
//
// Step 4: Build a service and type duration mapping by:
//         1. iterating over each span
//         2. add to the span's type and service duration the
//            duration portion
//         {
//             web-server: 15,
//             render: 15,
//             pg: 12.5,
//             pg-read: 15,
//             redis: 27.5,
//             rpc1: 30,
//             alert: 40,
//         }
//         {
//             web: 70,
//             cache: 55,
//             db: 55,
//             rpc: 55,
//         }
func (c *SublayerCalculator) ComputeSublayers(trace pb.Trace) []SublayerValue {
	c.reset(len(trace))
	c.computeParentIdx(trace)
	c.buildTimestamps(trace)
	nActiveSpans := 0
	activate := func(spanIdx int) {
		if c.activeSpansIndex[spanIdx] == -1 {
			c.activeSpansIndex[spanIdx] = nActiveSpans
			c.activeSpans[nActiveSpans] = spanIdx
			nActiveSpans++
		}
	}

	deactivate := func(spanIdx int) {
		i := c.activeSpansIndex[spanIdx]
		if i != -1 {
			c.activeSpansIndex[c.activeSpans[nActiveSpans-1]] = i
			c.activeSpans[i] = c.activeSpans[nActiveSpans-1]
			nActiveSpans--
			c.activeSpansIndex[spanIdx] = -1
		}
	}

	var previousTs int64

	for j := 0; j < len(trace)*2; j++ {
		tp := c.timestamps[j]
		var timeIncr float64
		if nActiveSpans > 0 {
			timeIncr = float64(tp.ts - previousTs)
		}
		previousTs = tp.ts
		if timeIncr > 0 {
			timeIncr /= float64(nActiveSpans)
			for i := 0; i < nActiveSpans; i++ {
				c.execDuration[c.activeSpans[i]] += timeIncr
			}
		}
		if tp.spanStart {
			deactivate(tp.parentIdx)
			c.openedSpans[tp.spanIdx] = true
			if c.nChildren[tp.spanIdx] == 0 {
				activate(tp.spanIdx)
			}
			c.nChildren[tp.parentIdx]++
		} else {
			c.openedSpans[tp.spanIdx] = false
			deactivate(tp.spanIdx)
			c.nChildren[tp.parentIdx]--
			if c.openedSpans[tp.parentIdx] && c.nChildren[tp.parentIdx] == 0 {
				activate(tp.parentIdx)
			}
		}
	}
	durationsByService := c.computeDurationByAttrNew(
		trace, func(s *pb.Span) string { return s.Service },
	)
	durationsByType := c.computeDurationByAttrNew(
		trace, func(s *pb.Span) string { return s.Type },
	)

	// Generate sublayers values
	values := make([]SublayerValue, 0,
		len(durationsByService)+len(durationsByType)+1,
	)

	for service, duration := range durationsByService {
		values = append(values, SublayerValue{
			Metric: "_sublayers.duration.by_service",
			Tag:    Tag{"sublayer_service", service},
			Value:  float64(int64(duration)),
		})
	}

	for spanType, duration := range durationsByType {
		values = append(values, SublayerValue{
			Metric: "_sublayers.duration.by_type",
			Tag:    Tag{"sublayer_type", spanType},
			Value:  float64(int64(duration)),
		})
	}

	values = append(values, SublayerValue{
		Metric: "_sublayers.span_count",
		Value:  float64(len(trace)),
	})

	return values
}

// attrSelector is used by computeDurationByAttr and is a func
// returning an attribute for a given span
type attrSelector func(*pb.Span) string

func (c *SublayerCalculator) computeDurationByAttrNew(trace pb.Trace, selector attrSelector) map[string]float64 {
	durations := make(map[string]float64)
	for i, span := range trace {
		key := selector(span)
		if key == "" {
			continue
		}
		durations[key] += c.execDuration[i]
	}
	return durations
}


// SetSublayersOnSpan takes some sublayers and pins them on the given span.Metrics
func SetSublayersOnSpan(span *pb.Span, values []SublayerValue) {
	if span.Metrics == nil {
		span.Metrics = make(map[string]float64, len(values))
	}

	for _, value := range values {
		name := value.Metric

		if value.Tag.Name != "" {
			name = name + "." + value.Tag.Name + ":" + value.Tag.Value
		}

		span.Metrics[name] = value.Value
	}
}
