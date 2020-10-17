// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package stats

import (
	"fmt"
	"math"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/trace/exportable/pb"
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

// spanState holds the state of span at a given timestamp in a trace.
type spanState struct {
	// open is true if the span has started, but hasn't ended yet.
	open bool
	// nOpenChildren is the number of direct children spans that are currently open
	nOpenChildren int
	// active is true if the span is open and has no open children.
	active bool
	// execDuration is the sum of the execution durations for the span since the start of the trace.
	// For each period where the span is active, its Exec Duration increases by periodDuration/numberOfActiveSpans
	//     so if a span is active during 20ms, in parallel with another span active at the same time, its exec duration will be 10ms
	execDuration float64
	// activationExecTime is the execTime of a trace at the time the span was activated.
	// When the span is deactivated, we increase the execDuration of the span by currentExecTime - activationExecTime
	activationExecTime float64
}

// activate activates a span and saves the given trace execution time at activation.
func (s *spanState) activate(execTime float64) {
	s.active = true
	s.activationExecTime = execTime
}

// deactivate marks a span as having become inactive at the given execution time.
func (s *spanState) deactivate(execTime float64) {
	s.active = false
	s.execDuration += execTime - s.activationExecTime
}

// SublayerCalculator holds arrays used to compute sublayer metrics.
// Re-using its fields between calls by sharing the same instance reduces allocations.
// A sublayer metric is the execution duration of a given type / service in a trace
// The metrics generated are detailed here: https://docs.datadoghq.com/tracing/guide/metrics_namespace/#duration-by
type SublayerCalculator struct {
	// spanStates holds the state of each span as the computeExecDurations traverses all the sorted timestamps of a trace.
	// It is indexed by the span index in the trace (for eg, spanState[0].open == true means that the first span of the trace is open)
	spanStates []spanState
	// timestamps are the sorted timestamps (starts and ends of each span) of a trace
	timestamps []timestamp
}

// NewSublayerCalculator returns a new SublayerCalculator.
func NewSublayerCalculator() *SublayerCalculator {
	s := &SublayerCalculator{}
	s.reset(defaultCalculatorSpanCapacity)
	return s
}

// reset clears structures of the sublayer calculator to prepare the computation of sublayer metrics for a trace with max n spans.
// if the capacity of the calculator is too small, it re-allocates new arrays
func (s *SublayerCalculator) reset(n int) {
	if n > len(s.spanStates) {
		s.spanStates = make([]spanState, n)
		s.timestamps = make([]timestamp, 2*n)
	}
	for i := 0; i < n; i++ {
		s.spanStates[i].open = false
		s.spanStates[i].nOpenChildren = 0
		s.spanStates[i].active = false
		s.spanStates[i].execDuration = 0
	}
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
// The algorithm consists of 3 steps:
// 1. Build the mapping from span ID --> span Index
// 2. Build the array of timestamps to consider (the start and ends of each span) and sort the array
// 3. Traverse the timestamps, and build the execution duration of each span during the traversal.
//    For each timestamp:
//    - Increase the trace exec duration by (previousPeriodDuration / numberOfActiveSpans)
//    - Update the span states
func (s *SublayerCalculator) computeExecDurations(trace pb.Trace) {
	// Step 1: Build the mapping spanID --> spanIdx
	idToIdx := make(map[uint64]int, len(trace))
	for idx, span := range trace {
		idToIdx[span.SpanID] = idx
	}

	// Step 2: Build all trace timestamps and sort them
	for i, span := range trace {
		parentIdx := -1
		if idx, ok := idToIdx[span.ParentID]; ok {
			parentIdx = idx
		}
		s.timestamps[2*i] = timestamp{spanStart: true, spanIdx: i, parentIdx: parentIdx, ts: span.Start}
		s.timestamps[2*i+1] = timestamp{spanStart: false, spanIdx: i, parentIdx: parentIdx, ts: span.Start + span.Duration}
	}
	sort.Sort(sortableTimestamps(s.timestamps[:2*len(trace)]))

	// Step 3: Compute the execution duration of each span
	state := s.spanStates
	// execTime is the execution time since the start of the trace.
	execTime := float64(0)
	nActiveSpans := 0
	for j := 0; j < len(trace)*2; j++ {
		tp := s.timestamps[j]
		if nActiveSpans > 0 {
			execTime += float64(tp.ts-s.timestamps[j-1].ts) / float64(nActiveSpans)
		}
		if tp.spanStart {
			// a span opens here
			if tp.parentIdx != -1 {
				// which has a parent
				if state[tp.parentIdx].active {
					// deactivate the parent
					state[tp.parentIdx].deactivate(execTime)
					nActiveSpans--
				}
				state[tp.parentIdx].nOpenChildren++
			}
			state[tp.spanIdx].open = true
			if state[tp.spanIdx].nOpenChildren == 0 && !state[tp.spanIdx].active {
				// activate the span
				state[tp.spanIdx].activate(execTime)
				nActiveSpans++
			}
		} else {
			// a span closes
			state[tp.spanIdx].open = false
			if state[tp.spanIdx].active {
				// deactivate the span
				state[tp.spanIdx].deactivate(execTime)
				nActiveSpans--
			}
			if tp.parentIdx != -1 {
				// which has a parent
				state[tp.parentIdx].nOpenChildren--
				if state[tp.parentIdx].open && state[tp.parentIdx].nOpenChildren == 0 && !state[tp.parentIdx].active {
					// reactivate the parent
					state[tp.parentIdx].activate(execTime)
					nActiveSpans++
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
// id 1: service=web-server, type=web,   parent=nil
// id 2: service=pg,         type=db,    parent=1
// id 3: service=render,     type=web,   parent=1
// id 4: service=pg-read,    type=db,    parent=2
// id 5: service=redis,      type=cache, parent=1
// id 6: service=rpc1,       type=rpc,   parent=1
// id 7: service=alert,      type=rpc,   parent=6
//
// Step 1: Compute the exec duration of each span of the trace.
// For a period of time when a span is active, the exec duration is defined as: periodDuration / numberOfActiveSpans during that period
//
//         {
//             spanID 1: 10/1 (between tp 0 and 10) + 10/2 (between tp 120 and 130) = 15,
//             ...
//             spanID 7: 10/2 (between tp 110 and 120) + 10/2 (between tp 120 and 130) + 20/1 (between tp 130 and 150),
//         }
//
// Step 2: Build a service and type duration mapping by:
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
func (s *SublayerCalculator) ComputeSublayers(trace pb.Trace) []SublayerValue {
	s.reset(len(trace))
	s.computeExecDurations(trace)
	durationsByService := s.computeDurationByAttr(
		trace, func(s *pb.Span) string { return s.Service },
	)
	durationsByType := s.computeDurationByAttr(
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
			Value:  math.Round(duration),
		})
	}

	for spanType, duration := range durationsByType {
		values = append(values, SublayerValue{
			Metric: "_sublayers.duration.by_type",
			Tag:    Tag{"sublayer_type", spanType},
			Value:  math.Round(duration),
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

func (s *SublayerCalculator) computeDurationByAttr(trace pb.Trace, selector attrSelector) map[string]float64 {
	durations := make(map[string]float64)
	for i, span := range trace {
		key := selector(span)
		if key == "" {
			continue
		}
		durations[key] += s.spanStates[i].execDuration
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
