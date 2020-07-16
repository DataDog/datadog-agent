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

const (
	// defaultSublayersCalculatorMaxSpans is the maximum trace size in spans the calculator can process.
	// if a bigger trace comes, the calculator will re-allocate bigger arrays.
	defaultSublayersCalculatorMaxSpans = 10000
	// unknownSpan represents a span that is not in the trace
	unknownSpan = -1
	// inactiveSpan is when a span is not active (open and with no direct open children, see ComputeSublayers)
	inactiveSpan = -1
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

// SublayerCalculator holds arrays used to compute sublayer metrics.
// Re-using arrays reduces the number of allocations
type SublayerCalculator struct {
	activeSpans      []int
	activeSpansIndex []int
	openSpans        []bool
	nChildren        []int
	execDuration     []float64
	parentIdx        []int
	timestamps       timestampArray
	maxSpans         int
}

// NewSublayerCalculator returns a new SublayerCalculator.
func NewSublayerCalculator() *SublayerCalculator {
	c := &SublayerCalculator{}
	c.initFields(defaultSublayersCalculatorMaxSpans)
	return c
}

// initFields initialized all arrays of the sublayer calculator
// it should be called every time we receive a trace with more than maxSpans spans
func (c *SublayerCalculator) initFields(maxSpans int) {
	c.maxSpans = maxSpans
	c.activeSpans = make([]int, maxSpans)
	c.activeSpansIndex = make([]int, maxSpans)
	c.openSpans = make([]bool, maxSpans)
	c.nChildren = make([]int, maxSpans)
	c.execDuration = make([]float64, maxSpans)
	c.parentIdx = make([]int, maxSpans)
	c.timestamps = make(timestampArray, 2*maxSpans)
}

// reset initializes structures of the sublayer calculator to prepare the computation of sublayer metrics
// for a trace with max n spans
func (c *SublayerCalculator) reset(n int) {
	if c.maxSpans < n {
		c.initFields(n)
	}
	for i := 0; i < n; i++ {
		c.activeSpansIndex[i] = inactiveSpan
		c.activeSpans[i] = 0
		c.openSpans[i] = false
		c.nChildren[i] = 0
		c.execDuration[i] = 0
	}
}

// a timestamp is the start or end of a span
// we store the span and parent index for each timestamp instead
// of storing a pointer to the span to be able to reuse the sublayer calculator arrays
type timestamp struct {
	spanStart bool
	spanIdx   int
	parentIdx int
	ts        int64
}

// timestampArray is an array of timestamp. Used to define Len, Swap and Less to be able to sort the array
type timestampArray []timestamp

func (t timestampArray) Len() int      { return len(t) }
func (t timestampArray) Swap(i, j int) { t[i], t[j] = t[j], t[i] }

// for spans with a duration of 0, we need to open them before closing them
func (t timestampArray) Less(i, j int) bool {
	return t[i].ts < t[j].ts || (t[i].ts == t[j].ts && t[i].spanStart && !t[j].spanStart)
}

// buildTimestamps builds puts starts and ends of each span in an array of timestamps, and sorts the array.
// result if stored in the SublayerCalculator
func (c *SublayerCalculator) buildTimestamps(trace pb.Trace) {
	for i, span := range trace {
		c.timestamps[2*i] = timestamp{spanStart: true, spanIdx: i, parentIdx: c.parentIdx[i], ts: span.Start}
		c.timestamps[2*i+1] = timestamp{spanStart: false, spanIdx: i, parentIdx: c.parentIdx[i], ts: span.Start + span.Duration}
	}
	sort.Sort(c.timestamps[:2*len(trace)])
}

// computeParentIdx builds the mapping spanIdx --> parentIdx
// it stores the result in the SublayerCalculator
func (c *SublayerCalculator) computeParentIdx(trace pb.Trace) {
	idToIdx := make(map[uint64]int, len(trace))
	for idx, span := range trace {
		idToIdx[span.SpanID] = idx
	}
	for idx, span := range trace {
		if parentIdx, ok := idToIdx[span.ParentID]; ok {
			c.parentIdx[idx] = parentIdx
		} else {
			c.parentIdx[idx] = unknownSpan
		}
	}
}

// computeExecDuration computes the exec duration of each span in the trace
//
// the algorithm does a traversal of all ordered timestamps (start and end of each span) in the trace
// Let's define a few terms to make the algorithm clearer.
// For a given timestamp:
//
// Open Span: The Span has started before this timestamp, and ends after this timestamp.
// Number of Children of a Span: At a given timestamp, number of direct children spans that are currently open
// Active Span: a Span is active at a given timestamp, if it is open and if it has no open children at the timestamp
// Exec duration: for each period where the span is active, its Exec Duration increases by deltaT/numberOfActiveSpans
//     so if a span is active during 20ms, in parallel with an other span active at the same time, its exec duration will be 10ms
//
// The algorithm goes through all the timestamps of the trace. During this traversal, it needs to:
// Keep track of which are the open spans
// Keep track of the active spans (open, and with no direct open children)
// Keep track of the number of children for each span at any given timestamp
// For each period between two timestamps, it knows which were the active spans. And it increases the exec duration for them by
// (interval duration / nActiveSpans)
func (c *SublayerCalculator) computeExecDurations(nSpans int) {
	nActiveSpans := 0
	// to store the active spans, we could use a set of span Indexes. But in order to re-use arrays and reduce allocations,
	// we use two arrays
	// in activeSpans, we store the active spans indexes. If span 0, 2 and 5 are active, activeSpans = [0, 2, 5, ...]
	// in activeSpansIndexes, we store the index in activeSpans of each span. For this case, activeSpansIndexes[0] = 0,
	// activeSpansIndexes[2] = 1 and activeSpansIndexes[5] = 2
	activate := func(spanIdx int) {
		if c.activeSpansIndex[spanIdx] == inactiveSpan {
			c.activeSpansIndex[spanIdx] = nActiveSpans
			c.activeSpans[nActiveSpans] = spanIdx
			nActiveSpans++
		}
	}

	deactivate := func(spanIdx int) {
		i := c.activeSpansIndex[spanIdx]
		if i != inactiveSpan {
			c.activeSpansIndex[c.activeSpans[nActiveSpans-1]] = i
			c.activeSpans[i] = c.activeSpans[nActiveSpans-1]
			nActiveSpans--
			c.activeSpansIndex[spanIdx] = inactiveSpan
		}
	}

	var previousTs int64

	for j := 0; j < nSpans*2; j++ {
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
			if tp.parentIdx != unknownSpan {
				deactivate(tp.parentIdx)
				c.nChildren[tp.parentIdx]++
			}
			c.openSpans[tp.spanIdx] = true
			if c.nChildren[tp.spanIdx] == 0 {
				activate(tp.spanIdx)
			}
		} else {
			c.openSpans[tp.spanIdx] = false
			deactivate(tp.spanIdx)
			if tp.parentIdx != unknownSpan {
				c.nChildren[tp.parentIdx]--
				if c.openSpans[tp.parentIdx] && c.nChildren[tp.parentIdx] == 0 {
					activate(tp.parentIdx)
				}
			}
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
//         of spans). For each timestamp, store the spanIdx, parentIdx and if it's the start or end of a span.
// if the parent span is not in the trace, parent idx is set to unknownSpan
//
//         [(0, span start, span idx 0, parent idx 7), (10, span start, span idx 1, parent idx 0), ...
//             (150, span end, span idx 6, parent idx 5)]
//
// Step 2: Compute the exec duration of each span of the trace.
// For a period of time when a span is active, the exec duration is defined as: duration/number of active spans during that period
//
//         {
//             idx 0 (spanID 1): 10/1 (between tp 0 and 10) + 10/2 (between tp 120 and 130) = 15,
//             ...
//             idx 6 (spanID 7): 10/2 (between tp 110 and 120) + 10/2 (between tp 120 and 130) + 20/1 (between tp 130 and 150),
//         }
//
// Step 3: Build a service and type duration mapping by:
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
	c.computeExecDurations(len(trace))
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
