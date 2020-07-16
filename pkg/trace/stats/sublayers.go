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
	// defaultCalculatorSpanCapacity specifies the maximum trace size in spans that the calculator
	// can process without re-allocating.
	defaultCalculatorSpanCapacity = 10000
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

// activeSpans is a set of active spans. We could use a map of span Indexes for that.
// But in order to re-use arrays and reduce allocations, we use two arrays.
// in spans, we store the active spans indexes. If span 0, 2 and 5 are active, spans = [0, 2, 5, ...]
// in indexes, we store the index in activeSpans of each span. For this case, indexes[0] = 0,
// indexes[2] = 1 and indexes[5] = 2
type activeSpanSet struct {
	// capacity of the set. Also, the set can only contain numbers in [0;capacity[
	capacity int
	// size of the set. Current number of elements
	size int
	// spans are the elements of the set
	spans []int
	// indexes[i] contains the index in spans of element i. It is used when removing elements
	indexes []int
}

// add adds element to the set if it's not already in it
func (a *activeSpanSet) add(spanIdx int) {
	if a.indexes[spanIdx] == -1 {
		a.indexes[spanIdx] = a.size
		a.spans[a.size] = spanIdx
		a.size++
	}
}

// remove removes element from the set if it's in it
func (a *activeSpanSet) remove(spanIdx int) {
	i := a.indexes[spanIdx]
	if i != -1 {
		a.indexes[a.spans[a.size-1]] = i
		a.spans[i] = a.spans[a.size-1]
		a.size--
		a.indexes[spanIdx] = -1
	}

}

// clear clears the set. It prepares it to hold maximum n elements
func (a *activeSpanSet) clear(n int) {
	a.size = 0
	for i := 0; i < n; i++ {
		a.indexes[i] = -1
		a.spans[i] = 0
	}
}

// resize updates the capacity of the set
func (a *activeSpanSet) resize(capacity int) {
	a.capacity = capacity
	a.spans = make([]int, capacity)
	a.indexes = make([]int, capacity)
}

// SublayerCalculator holds arrays used to compute sublayer metrics.
// A sublayer metric is the execution duration a given type / service takes of a trace
// Re-using arrays reduces the number of allocations
type SublayerCalculator struct {
	// openSpans holds whether each span is opened. A span is opened if it has started, but hasn't ended yet.
	openSpans []bool
	// nChildren is the Number of Children of a Span: At a given timestamp, number of direct children spans that are currently open
	nChildren []int
	// activeSpans is a set active spans. A span is active at a given timestamp, if it is open and if it has no open children.
	activeSpans *activeSpanSet
	// Exec duration: for each period where the span is active, its Exec Duration increases by deltaT/numberOfActiveSpans
	//     so if a span is active during 20ms, in parallel with an other span active at the same time, its exec duration will be 10ms
	execDuration []float64
	parentIdx    []int
	timestamps   sortableTimestamps
	capacity     int
}

// NewSublayerCalculator returns a new SublayerCalculator.
func NewSublayerCalculator() *SublayerCalculator {
	c := &SublayerCalculator{}
	c.resize(defaultCalculatorSpanCapacity)
	return c
}

// resize initialized allocates arrays of the sublayer calculator
// it should be called every time we receive a trace with more spans than the capacity of the calculator
func (c *SublayerCalculator) resize(capacity int) {
	c.capacity = capacity
	c.openSpans = make([]bool, capacity)
	c.nChildren = make([]int, capacity)
	c.execDuration = make([]float64, capacity)
	c.parentIdx = make([]int, capacity)
	c.timestamps = make(sortableTimestamps, 2*capacity)
	c.activeSpans.resize(capacity)
}

// clear clears structures of the sublayer calculator to prepare the computation of sublayer metrics
// for a trace with max n spans
func (c *SublayerCalculator) clear(n int) {
	for i := 0; i < n; i++ {
		c.openSpans[i] = false
		c.nChildren[i] = 0
		c.execDuration[i] = 0
	}
	c.activeSpans.clear(n)
}

// timestamp stores a point in time where a span might have started or ended.
// It contains additionally information such as its own and its parent's indexes
// in a SublayerCalculator.
type timestamp struct {
	// spanStart specifies whether this timestamp is the start of a span.
	spanStart bool
	// spanIdx specifies the index of the span in the SublayerCalculator.
	spanIdx int
	// parentIdx specifies the index of this span's parent in the SublayerCalculator.
	parentIdx int
	// ts is the actual timestamp, as given by (time.Time).UnixNano()
	ts int64
}

// sortableTimestamps implements sort.Sort on top of a slice of timestamps.
type sortableTimestamps []timestamp

func (t sortableTimestamps) Len() int      { return len(t) }
func (t sortableTimestamps) Swap(i, j int) { t[i], t[j] = t[j], t[i] }
func (t sortableTimestamps) Less(i, j int) bool {
	return t[i].ts < t[j].ts || (t[i].ts == t[j].ts && t[i].spanStart && !t[j].spanStart)
}

// computeExecDuration computes the exec duration of each span in the trace
//
// the algorithm does a traversal of all ordered timestamps (start and end of each span) in the trace
// during this traversal, it needs to:
// Keep track of which are the open spans
// Keep track of the active spans (open, and with no direct open children)
// Keep track of the number of children for each span at any given timestamp
// For each period between two timestamps, it knows which were the active spans. And it increases the exec duration for them by
// (interval duration / nActiveSpans)
func (c *SublayerCalculator) computeExecDurations(trace pb.Trace) {
	// builds the mapping spanIdx --> parentIdx
	idToIdx := make(map[uint64]int, len(trace))
	for idx, span := range trace {
		idToIdx[span.SpanID] = idx
	}
	for idx, span := range trace {
		if parentIdx, ok := idToIdx[span.ParentID]; ok {
			c.parentIdx[idx] = parentIdx
		} else {
			c.parentIdx[idx] = -1
		}
	}

	// build all trace timestamps and sorts them
	for i, span := range trace {
		c.timestamps[2*i] = timestamp{spanStart: true, spanIdx: i, parentIdx: c.parentIdx[i], ts: span.Start}
		c.timestamps[2*i+1] = timestamp{spanStart: false, spanIdx: i, parentIdx: c.parentIdx[i], ts: span.Start + span.Duration}
	}
	sort.Sort(c.timestamps[:2*len(trace)])

	// compute execution duration of each span
	for j := 0; j < len(trace)*2; j++ {
		tp := c.timestamps[j]
		var timeIncr float64
		if c.activeSpans.size > 0 {
			timeIncr = float64(tp.ts - c.timestamps[j-1].ts)
		}
		if timeIncr > 0 {
			timeIncr /= float64(c.activeSpans.size)
			for i := 0; i < c.activeSpans.size; i++ {
				c.execDuration[c.activeSpans.spans[i]] += timeIncr
			}
		}
		if tp.spanStart {
			if tp.parentIdx != -1 {
				c.activeSpans.remove(tp.parentIdx)
				c.nChildren[tp.parentIdx]++
			}
			c.openSpans[tp.spanIdx] = true
			if c.nChildren[tp.spanIdx] == 0 {
				c.activeSpans.add(tp.spanIdx)
			}
		} else {
			c.openSpans[tp.spanIdx] = false
			c.activeSpans.remove(tp.spanIdx)
			if tp.parentIdx != -1 {
				c.nChildren[tp.parentIdx]--
				if c.openSpans[tp.parentIdx] && c.nChildren[tp.parentIdx] == 0 {
					c.activeSpans.add(tp.parentIdx)
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
// if the parent span is not in the trace, parent idx is set to -1
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
	if len(trace) > c.capacity {
		c.resize(len(trace))
	}
	c.clear(len(trace))
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
