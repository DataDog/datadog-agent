// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"sync"

	flatbuffers "github.com/google/flatbuffers/go"

	signals "github.com/DataDog/datadog-agent/comp/flightrecorder/impl/signals"
)

// logEntry is a compact log entry for known contexts (fast path).
type logEntry struct {
	ContextKey  uint64
	Content     []byte
	TimestampNs int64
}

// metricPoint is a compact data point for known contexts (fast path).
type metricPoint struct {
	ContextKey  uint64
	Value       float64
	TimestampNs int64
	SampleRate  float64
}

// contextDef is a context definition (slow path, first occurrence only).
type contextDef struct {
	ContextKey   uint64
	Name         string
	Tags         []string
	TagPoolSlice *[]string // non-nil when Tags was borrowed from tagPool
	Source       string
}

// capturedTraceStat is an internal copy of a trace stats entry.
type capturedTraceStat struct {
	Service          string
	Name             string
	Resource         string
	Type             string
	SpanKind         string
	HTTPStatusCode   uint32
	Hits             uint64
	Errors           uint64
	DurationNs       uint64
	TopLevelHits     uint64
	OkSummary        []byte
	ErrorSummary     []byte
	Hostname         string
	Env              string
	Version          string
	BucketStartNs    int64
	BucketDurationNs int64
	TimestampNs      int64
}


// ---------------------------------------------------------------------------
// Builder pool
// ---------------------------------------------------------------------------

type builderPool struct {
	builders sync.Pool
	offsets  sync.Pool
}

const maxRetainedBuilderBytes = 256 * 1024

func newBuilderPool() *builderPool {
	return &builderPool{
		builders: sync.Pool{New: func() any { return flatbuffers.NewBuilder(4096) }},
		offsets:  sync.Pool{New: func() any { s := make([]flatbuffers.UOffsetT, 0, 256); return &s }},
	}
}

func (p *builderPool) get() *flatbuffers.Builder {
	return p.builders.Get().(*flatbuffers.Builder)
}

func (p *builderPool) put(b *flatbuffers.Builder) {
	if cap(b.Bytes) > maxRetainedBuilderBytes {
		return
	}
	b.Reset()
	p.builders.Put(b)
}

func (p *builderPool) getOffsets(n int) (*[]flatbuffers.UOffsetT, []flatbuffers.UOffsetT) {
	sp := p.offsets.Get().(*[]flatbuffers.UOffsetT)
	s := *sp
	if cap(s) < n {
		s = make([]flatbuffers.UOffsetT, n)
	} else {
		s = s[:n]
	}
	return sp, s
}

func (p *builderPool) putOffsets(sp *[]flatbuffers.UOffsetT, s []flatbuffers.UOffsetT) {
	*sp = s[:0]
	p.offsets.Put(sp)
}

// ---------------------------------------------------------------------------
// Context batch encoding
// ---------------------------------------------------------------------------

// EncodeContextBatch encodes context definitions from a ring buffer segment
// into a SignalEnvelope with a MetricBatch containing only the contexts vector.
func EncodeContextBatch(
	pool *builderPool,
	defs []contextDef, tail, count, capacity int,
) (*flatbuffers.Builder, error) {
	b := pool.get()

	var tagBuf []flatbuffers.UOffsetT
	offSp, ctxOffsets := pool.getOffsets(count)

	for ri := count - 1; ri >= 0; ri-- {
		idx := (tail + ri) % capacity
		d := &defs[idx]

		nameOff := b.CreateString(d.Name)
		var sourceOff flatbuffers.UOffsetT
		if d.Source != "" {
			sourceOff = b.CreateSharedString(d.Source)
		}
		if cap(tagBuf) < len(d.Tags) {
			tagBuf = make([]flatbuffers.UOffsetT, len(d.Tags))
		} else {
			tagBuf = tagBuf[:len(d.Tags)]
		}
		for j := len(d.Tags) - 1; j >= 0; j-- {
			tagBuf[j] = b.CreateString(d.Tags[j])
		}
		signals.ContextEntryStartTagsVector(b, len(d.Tags))
		for j := len(tagBuf) - 1; j >= 0; j-- {
			b.PrependUOffsetT(tagBuf[j])
		}
		tagsVec := b.EndVector(len(d.Tags))

		signals.ContextEntryStart(b)
		signals.ContextEntryAddContextKey(b, d.ContextKey)
		signals.ContextEntryAddName(b, nameOff)
		signals.ContextEntryAddTags(b, tagsVec)
		if sourceOff != 0 {
			signals.ContextEntryAddSource(b, sourceOff)
		}
		ctxOffsets[ri] = signals.ContextEntryEnd(b)
	}

	signals.MetricBatchStartContextsVector(b, count)
	for i := len(ctxOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(ctxOffsets[i])
	}
	contextsVec := b.EndVector(count)
	pool.putOffsets(offSp, ctxOffsets)

	signals.MetricBatchStart(b)
	signals.MetricBatchAddContexts(b, contextsVec)
	batchOff := signals.MetricBatchEnd(b)

	signals.SignalEnvelopeStart(b)
	signals.SignalEnvelopeAddMetricBatch(b, batchOff)
	envOff := signals.SignalEnvelopeEnd(b)

	b.Finish(envOff)
	return b, nil
}

// ---------------------------------------------------------------------------
// Point batch encoding
// ---------------------------------------------------------------------------

// EncodePointBatch encodes metric data points from a ring buffer segment
// into a SignalEnvelope with a MetricBatch containing only the points vector.
func EncodePointBatch(
	pool *builderPool,
	pts []metricPoint, tail, count, capacity int,
) (*flatbuffers.Builder, error) {
	b := pool.get()

	offSp, ptOffsets := pool.getOffsets(count)

	for ri := count - 1; ri >= 0; ri-- {
		idx := (tail + ri) % capacity
		p := &pts[idx]

		signals.PointEntryStart(b)
		signals.PointEntryAddContextKey(b, p.ContextKey)
		signals.PointEntryAddValue(b, p.Value)
		signals.PointEntryAddTimestampNs(b, p.TimestampNs)
		signals.PointEntryAddSampleRate(b, p.SampleRate)
		ptOffsets[ri] = signals.PointEntryEnd(b)
	}

	signals.MetricBatchStartPointsVector(b, count)
	for i := len(ptOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(ptOffsets[i])
	}
	pointsVec := b.EndVector(count)
	pool.putOffsets(offSp, ptOffsets)

	signals.MetricBatchStart(b)
	signals.MetricBatchAddPoints(b, pointsVec)
	batchOff := signals.MetricBatchEnd(b)

	signals.SignalEnvelopeStart(b)
	signals.SignalEnvelopeAddMetricBatch(b, batchOff)
	envOff := signals.SignalEnvelopeEnd(b)

	b.Finish(envOff)
	return b, nil
}

// ---------------------------------------------------------------------------
// Log batch encoding
// ---------------------------------------------------------------------------

// EncodeLogContextBatch encodes log context definitions from a ring buffer
// segment into a SignalEnvelope with a LogBatch containing only the contexts
// vector. Mirrors EncodeContextBatch but wraps in LogBatch instead of
// MetricBatch so the sidecar routes it to the logs writer thread.
func EncodeLogContextBatch(
	pool *builderPool,
	defs []contextDef, tail, count, capacity int,
) (*flatbuffers.Builder, error) {
	b := pool.get()

	var tagBuf []flatbuffers.UOffsetT
	offSp, ctxOffsets := pool.getOffsets(count)

	for ri := count - 1; ri >= 0; ri-- {
		idx := (tail + ri) % capacity
		d := &defs[idx]

		nameOff := b.CreateString(d.Name)
		var sourceOff flatbuffers.UOffsetT
		if d.Source != "" {
			sourceOff = b.CreateSharedString(d.Source)
		}
		if cap(tagBuf) < len(d.Tags) {
			tagBuf = make([]flatbuffers.UOffsetT, len(d.Tags))
		} else {
			tagBuf = tagBuf[:len(d.Tags)]
		}
		for j := len(d.Tags) - 1; j >= 0; j-- {
			tagBuf[j] = b.CreateString(d.Tags[j])
		}
		signals.ContextEntryStartTagsVector(b, len(d.Tags))
		for j := len(tagBuf) - 1; j >= 0; j-- {
			b.PrependUOffsetT(tagBuf[j])
		}
		tagsVec := b.EndVector(len(d.Tags))

		signals.ContextEntryStart(b)
		signals.ContextEntryAddContextKey(b, d.ContextKey)
		signals.ContextEntryAddName(b, nameOff)
		signals.ContextEntryAddTags(b, tagsVec)
		if sourceOff != 0 {
			signals.ContextEntryAddSource(b, sourceOff)
		}
		ctxOffsets[ri] = signals.ContextEntryEnd(b)
	}

	signals.LogBatchStartContextsVector(b, count)
	for i := len(ctxOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(ctxOffsets[i])
	}
	contextsVec := b.EndVector(count)
	pool.putOffsets(offSp, ctxOffsets)

	signals.LogBatchStart(b)
	signals.LogBatchAddContexts(b, contextsVec)
	batchOff := signals.LogBatchEnd(b)

	signals.SignalEnvelopeStart(b)
	signals.SignalEnvelopeAddLogBatch(b, batchOff)
	envOff := signals.SignalEnvelopeEnd(b)

	b.Finish(envOff)
	return b, nil
}

// EncodeLogEntryBatch encodes compact log entries from a ring buffer segment
// into a SignalEnvelope with a LogBatch containing only the entries vector.
func EncodeLogEntryBatch(pool *builderPool, buf []logEntry, tail, count, capacity int) (*flatbuffers.Builder, error) {
	b := pool.get()

	offSp, entryOffsets := pool.getOffsets(count)
	for ri := count - 1; ri >= 0; ri-- {
		idx := (tail + ri) % capacity
		e := &buf[idx]

		contentOff := b.CreateByteVector(e.Content)

		signals.LogEntryStart(b)
		signals.LogEntryAddContextKey(b, e.ContextKey)
		signals.LogEntryAddContent(b, contentOff)
		signals.LogEntryAddTimestampNs(b, e.TimestampNs)
		entryOffsets[ri] = signals.LogEntryEnd(b)
	}

	signals.LogBatchStartEntriesVector(b, count)
	for i := len(entryOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(entryOffsets[i])
	}
	entriesVec := b.EndVector(count)
	pool.putOffsets(offSp, entryOffsets)

	signals.LogBatchStart(b)
	signals.LogBatchAddEntries(b, entriesVec)
	batchOff := signals.LogBatchEnd(b)

	signals.SignalEnvelopeStart(b)
	signals.SignalEnvelopeAddLogBatch(b, batchOff)
	envOff := signals.SignalEnvelopeEnd(b)

	b.Finish(envOff)
	return b, nil
}

// ---------------------------------------------------------------------------
// Trace stats batch encoding
// ---------------------------------------------------------------------------

// EncodeTraceStatsBatchRing encodes trace stats from a ring buffer segment
// into a SignalEnvelope with a TraceStatsBatch.
func EncodeTraceStatsBatchRing(pool *builderPool, buf []capturedTraceStat, tail, count, capacity int) (*flatbuffers.Builder, error) {
	b := pool.get()

	offSp, entryOffsets := pool.getOffsets(count)
	for ri := count - 1; ri >= 0; ri-- {
		idx := (tail + ri) % capacity
		e := &buf[idx]

		serviceOff := b.CreateSharedString(e.Service)
		nameOff := b.CreateSharedString(e.Name)
		resourceOff := b.CreateSharedString(e.Resource)
		typeOff := b.CreateSharedString(e.Type)
		spanKindOff := b.CreateSharedString(e.SpanKind)
		hostnameOff := b.CreateSharedString(e.Hostname)
		envOff := b.CreateSharedString(e.Env)
		versionOff := b.CreateSharedString(e.Version)

		var okSummaryOff flatbuffers.UOffsetT
		if len(e.OkSummary) > 0 {
			okSummaryOff = b.CreateByteVector(e.OkSummary)
		}
		var errorSummaryOff flatbuffers.UOffsetT
		if len(e.ErrorSummary) > 0 {
			errorSummaryOff = b.CreateByteVector(e.ErrorSummary)
		}

		signals.TraceStatEntryStart(b)
		signals.TraceStatEntryAddService(b, serviceOff)
		signals.TraceStatEntryAddName(b, nameOff)
		signals.TraceStatEntryAddResource(b, resourceOff)
		signals.TraceStatEntryAddType(b, typeOff)
		signals.TraceStatEntryAddSpanKind(b, spanKindOff)
		signals.TraceStatEntryAddHttpStatusCode(b, e.HTTPStatusCode)
		signals.TraceStatEntryAddHits(b, e.Hits)
		signals.TraceStatEntryAddErrors(b, e.Errors)
		signals.TraceStatEntryAddDurationNs(b, e.DurationNs)
		signals.TraceStatEntryAddTopLevelHits(b, e.TopLevelHits)
		if okSummaryOff != 0 {
			signals.TraceStatEntryAddOkSummary(b, okSummaryOff)
		}
		if errorSummaryOff != 0 {
			signals.TraceStatEntryAddErrorSummary(b, errorSummaryOff)
		}
		signals.TraceStatEntryAddHostname(b, hostnameOff)
		signals.TraceStatEntryAddEnv(b, envOff)
		signals.TraceStatEntryAddVersion(b, versionOff)
		signals.TraceStatEntryAddBucketStartNs(b, e.BucketStartNs)
		signals.TraceStatEntryAddBucketDurationNs(b, e.BucketDurationNs)
		signals.TraceStatEntryAddTimestampNs(b, e.TimestampNs)
		entryOffsets[ri] = signals.TraceStatEntryEnd(b)
	}

	signals.TraceStatsBatchStartEntriesVector(b, count)
	for i := len(entryOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(entryOffsets[i])
	}
	entriesVec := b.EndVector(count)
	pool.putOffsets(offSp, entryOffsets)

	signals.TraceStatsBatchStart(b)
	signals.TraceStatsBatchAddEntries(b, entriesVec)
	batchOff := signals.TraceStatsBatchEnd(b)

	signals.SignalEnvelopeStart(b)
	signals.SignalEnvelopeAddTraceStatsBatch(b, batchOff)
	envOff := signals.SignalEnvelopeEnd(b)

	b.Finish(envOff)
	return b, nil
}
