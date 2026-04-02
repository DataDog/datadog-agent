// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"encoding/binary"
	"net"
	"sync"

	flatbuffers "github.com/google/flatbuffers/go"

	signals "github.com/DataDog/datadog-agent/comp/flightrecorder/impl/signals"
	"github.com/DataDog/datadog-agent/pkg/hook"
)

// metricPoint is a compact data point for known contexts (fast path).
// This is what 99.9% of ring buffer items look like after context warm-up.
// Source is a static string constant (e.g. "dogstatsd-pipeline") — no
// heap allocation on the hot path.
type metricPoint struct {
	ContextKey  uint64
	Value       float64
	TimestampNs int64
	SampleRate  float64
	Source      string
}

// contextDef is a context definition (slow path, first occurrence only).
// Contains the full name + tags strings that the sidecar needs to resolve
// subsequent metricPoint references.
type contextDef struct {
	ContextKey   uint64
	Name         string
	Value        float64
	Tags         []string
	TagPoolSlice *[]string // non-nil when Tags was borrowed from tagPool
	TimestampNs  int64
	SampleRate   float64
	Source       string
}

// capturedMetric is kept for the legacy EncodeMetricBatch (tests/benchmarks).
type capturedMetric struct {
	ContextKey   uint64
	Name         string
	Value        float64
	Tags         []string
	TagPoolSlice *[]string
	TimestampNs  int64
	SampleRate   float64
	Source       string
}

// capturedTraceStat is an internal copy of a trace stats entry.
type capturedTraceStat struct {
	Service         string
	Name            string
	Resource        string
	Type            string
	SpanKind        string
	HTTPStatusCode  uint32
	Hits            uint64
	Errors          uint64
	DurationNs      uint64
	TopLevelHits    uint64
	OkSummary       []byte
	ErrorSummary    []byte
	Hostname        string
	Env             string
	Version         string
	BucketStartNs   int64
	BucketDurationNs int64
	TimestampNs     int64
}

// capturedLog is a type alias for hook.LogSampleSnapshot, used in encoder and batcher.
type capturedLog = hook.LogSampleSnapshot

// ---------------------------------------------------------------------------
// Builder pool (optimisation 1B)
// ---------------------------------------------------------------------------

// builderPool recycles FlatBuffers builders and offset slices between flushes.
type builderPool struct {
	builders sync.Pool
	offsets  sync.Pool // reusable []flatbuffers.UOffsetT slices
}

// maxRetainedBuilderBytes caps the builder's internal byte buffer.
// Builders that grew beyond this (from a one-off large batch) are discarded.
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
	// Check cap(Bytes) — the internal buffer capacity — not FinishedBytes length.
	// Reset() preserves cap(Bytes), so a builder that once grew to 5 MB stays
	// at 5 MB forever. Discard oversized builders to bound pool memory.
	if cap(b.Bytes) > maxRetainedBuilderBytes {
		return
	}
	b.Reset()
	p.builders.Put(b)
}

// getOffsets borrows a reusable offset slice from the pool, grown to at least n.
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

// putOffsets returns an offset slice to the pool.
func (p *builderPool) putOffsets(sp *[]flatbuffers.UOffsetT, s []flatbuffers.UOffsetT) {
	*sp = s[:0]
	p.offsets.Put(sp)
}

// ---------------------------------------------------------------------------
// Metric batch encoding
// ---------------------------------------------------------------------------

// EncodeMetricBatch serialises samples into a size-prefixed FlatBuffers SignalEnvelope(MetricBatch).
// It allocates a fresh builder (used by benchmarks and tests).
func EncodeMetricBatch(samples []capturedMetric) ([]byte, error) {
	b := flatbuffers.NewBuilder(1024)
	return encodeMetricBatchWith(b, samples)
}

// EncodeSplitMetricBatch encodes metrics from two ring buffers: context
// definitions (with strings) and data points (compact, no strings).
// Definitions are encoded first so the sidecar sees them before references.
// EncodeSplitMetricBatch encodes metrics from two ring buffers.
// Returns the builder with FinishSizePrefixed data. The caller must
// send b.FinishedBytes() then call pool.put(b) to recycle the builder.
func EncodeSplitMetricBatch(
	pool *builderPool,
	defs []contextDef, defTail, defCount, defCap int,
	pts []metricPoint, ptTail, ptCount, ptCap int,
) (*flatbuffers.Builder, error) {
	total := defCount + ptCount
	b := pool.get()

	var tagBuf []flatbuffers.UOffsetT
	offSp, sampleOffsets := pool.getOffsets(total)

	// Encode data points (references) first in reverse — they'll appear after
	// definitions in the final vector.
	for ri := ptCount - 1; ri >= 0; ri-- {
		idx := (ptTail + ri) % ptCap
		p := &pts[idx]
		var sourceOff flatbuffers.UOffsetT
		if p.Source != "" {
			sourceOff = b.CreateSharedString(p.Source)
		}
		signals.MetricSampleStart(b)
		signals.MetricSampleAddContextKey(b, p.ContextKey)
		signals.MetricSampleAddValue(b, p.Value)
		signals.MetricSampleAddTimestampNs(b, p.TimestampNs)
		signals.MetricSampleAddSampleRate(b, p.SampleRate)
		if sourceOff != 0 {
			signals.MetricSampleAddSource(b, sourceOff)
		}
		sampleOffsets[defCount+ri] = signals.MetricSampleEnd(b)
	}

	// Encode context definitions in reverse.
	for ri := defCount - 1; ri >= 0; ri-- {
		idx := (defTail + ri) % defCap
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
		signals.MetricSampleStartTagsVector(b, len(d.Tags))
		for j := len(tagBuf) - 1; j >= 0; j-- {
			b.PrependUOffsetT(tagBuf[j])
		}
		tagsVec := b.EndVector(len(d.Tags))

		signals.MetricSampleStart(b)
		signals.MetricSampleAddContextKey(b, d.ContextKey)
		signals.MetricSampleAddName(b, nameOff)
		signals.MetricSampleAddTags(b, tagsVec)
		signals.MetricSampleAddValue(b, d.Value)
		signals.MetricSampleAddTimestampNs(b, d.TimestampNs)
		signals.MetricSampleAddSampleRate(b, d.SampleRate)
		if sourceOff != 0 {
			signals.MetricSampleAddSource(b, sourceOff)
		}
		sampleOffsets[ri] = signals.MetricSampleEnd(b)
	}

	signals.MetricBatchStartSamplesVector(b, total)
	for i := len(sampleOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(sampleOffsets[i])
	}
	samplesVec := b.EndVector(total)
	pool.putOffsets(offSp, sampleOffsets)

	signals.MetricBatchStart(b)
	signals.MetricBatchAddSamples(b, samplesVec)
	batchOff := signals.MetricBatchEnd(b)

	signals.SignalEnvelopeStart(b)
	signals.SignalEnvelopeAddPayloadType(b, signals.SignalPayloadMetricBatch)
	signals.SignalEnvelopeAddPayload(b, batchOff)
	envOff := signals.SignalEnvelopeEnd(b)

	b.Finish(envOff)

	return b, nil
}

// encodeMetricBatchWith uses the provided builder.
func encodeMetricBatchWith(b *flatbuffers.Builder, samples []capturedMetric) ([]byte, error) {
	var tagBuf []flatbuffers.UOffsetT

	sampleOffsets := make([]flatbuffers.UOffsetT, len(samples))
	for i := len(samples) - 1; i >= 0; i-- {
		s := &samples[i]
		isRef := s.ContextKey != 0 && s.Name == ""

		var nameOff, sourceOff, tagsVec flatbuffers.UOffsetT
		if !isRef {
			nameOff = b.CreateString(s.Name)
			sourceOff = b.CreateString(s.Source)

			if cap(tagBuf) < len(s.Tags) {
				tagBuf = make([]flatbuffers.UOffsetT, len(s.Tags))
			} else {
				tagBuf = tagBuf[:len(s.Tags)]
			}
			for j := len(s.Tags) - 1; j >= 0; j-- {
				tagBuf[j] = b.CreateString(s.Tags[j])
			}
			signals.MetricSampleStartTagsVector(b, len(s.Tags))
			for j := len(tagBuf) - 1; j >= 0; j-- {
				b.PrependUOffsetT(tagBuf[j])
			}
			tagsVec = b.EndVector(len(s.Tags))
		}

		signals.MetricSampleStart(b)
		if !isRef {
			signals.MetricSampleAddName(b, nameOff)
			signals.MetricSampleAddTags(b, tagsVec)
			signals.MetricSampleAddSource(b, sourceOff)
		}
		signals.MetricSampleAddValue(b, s.Value)
		signals.MetricSampleAddTimestampNs(b, s.TimestampNs)
		signals.MetricSampleAddSampleRate(b, s.SampleRate)
		if s.ContextKey != 0 {
			signals.MetricSampleAddContextKey(b, s.ContextKey)
		}
		sampleOffsets[i] = signals.MetricSampleEnd(b)
	}

	signals.MetricBatchStartSamplesVector(b, len(samples))
	for i := len(sampleOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(sampleOffsets[i])
	}
	samplesVec := b.EndVector(len(samples))

	signals.MetricBatchStart(b)
	signals.MetricBatchAddSamples(b, samplesVec)
	batchOff := signals.MetricBatchEnd(b)

	signals.SignalEnvelopeStart(b)
	signals.SignalEnvelopeAddPayloadType(b, signals.SignalPayloadMetricBatch)
	signals.SignalEnvelopeAddPayload(b, batchOff)
	envOff := signals.SignalEnvelopeEnd(b)

	b.Finish(envOff)

	return sizePrefixed(b.FinishedBytes()), nil
}

// ---------------------------------------------------------------------------
// Log batch encoding
// ---------------------------------------------------------------------------

// EncodeLogBatch serialises entries into a size-prefixed FlatBuffers SignalEnvelope(LogBatch).
func EncodeLogBatch(entries []capturedLog) ([]byte, error) {
	b := flatbuffers.NewBuilder(1024)
	return encodeLogBatchWith(b, entries)
}

// EncodeLogBatchRing encodes logs directly from a ring buffer segment, using a pooled builder.
// EncodeLogBatchRing encodes logs directly from a ring buffer segment.
// Returns the builder with FinishSizePrefixed data. The caller must
// send b.FinishedBytes() then call pool.put(b) to recycle the builder.
func EncodeLogBatchRing(pool *builderPool, buf []capturedLog, tail, count, capacity int) (*flatbuffers.Builder, error) {
	b := pool.get()

	var tagBuf []flatbuffers.UOffsetT
	offSp, entryOffsets := pool.getOffsets(count)
	for ri := count - 1; ri >= 0; ri-- {
		idx := (tail + ri) % capacity
		e := &buf[idx]

		contentOff := b.CreateByteVector(e.Content)
		statusOff := b.CreateSharedString(e.Status)
		hostnameOff := b.CreateSharedString(e.Hostname)
		sourceOff := b.CreateSharedString("") // LogSampleSnapshot doesn't carry Source

		if cap(tagBuf) < len(e.Tags) {
			tagBuf = make([]flatbuffers.UOffsetT, len(e.Tags))
		} else {
			tagBuf = tagBuf[:len(e.Tags)]
		}
		for j := len(e.Tags) - 1; j >= 0; j-- {
			tagBuf[j] = b.CreateString(e.Tags[j])
		}
		signals.LogEntryStartTagsVector(b, len(e.Tags))
		for j := len(tagBuf) - 1; j >= 0; j-- {
			b.PrependUOffsetT(tagBuf[j])
		}
		tagsVec := b.EndVector(len(e.Tags))

		signals.LogEntryStart(b)
		signals.LogEntryAddContent(b, contentOff)
		signals.LogEntryAddStatus(b, statusOff)
		signals.LogEntryAddTags(b, tagsVec)
		signals.LogEntryAddHostname(b, hostnameOff)
		signals.LogEntryAddTimestampNs(b, e.TimestampNs)
		signals.LogEntryAddSource(b, sourceOff)
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
	signals.SignalEnvelopeAddPayloadType(b, signals.SignalPayloadLogBatch)
	signals.SignalEnvelopeAddPayload(b, batchOff)
	envOff := signals.SignalEnvelopeEnd(b)

	b.Finish(envOff)

	return b, nil
}

func encodeLogBatchWith(b *flatbuffers.Builder, entries []capturedLog) ([]byte, error) {
	var tagBuf []flatbuffers.UOffsetT
	entryOffsets := make([]flatbuffers.UOffsetT, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		e := &entries[i]

		contentOff := b.CreateByteVector(e.Content)
		statusOff := b.CreateString(e.Status)
		hostnameOff := b.CreateString(e.Hostname)
		sourceOff := b.CreateString("")

		if cap(tagBuf) < len(e.Tags) {
			tagBuf = make([]flatbuffers.UOffsetT, len(e.Tags))
		} else {
			tagBuf = tagBuf[:len(e.Tags)]
		}
		for j := len(e.Tags) - 1; j >= 0; j-- {
			tagBuf[j] = b.CreateString(e.Tags[j])
		}
		signals.LogEntryStartTagsVector(b, len(e.Tags))
		for j := len(tagBuf) - 1; j >= 0; j-- {
			b.PrependUOffsetT(tagBuf[j])
		}
		tagsVec := b.EndVector(len(e.Tags))

		signals.LogEntryStart(b)
		signals.LogEntryAddContent(b, contentOff)
		signals.LogEntryAddStatus(b, statusOff)
		signals.LogEntryAddTags(b, tagsVec)
		signals.LogEntryAddHostname(b, hostnameOff)
		signals.LogEntryAddTimestampNs(b, e.TimestampNs)
		signals.LogEntryAddSource(b, sourceOff)
		entryOffsets[i] = signals.LogEntryEnd(b)
	}

	signals.LogBatchStartEntriesVector(b, len(entries))
	for i := len(entryOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(entryOffsets[i])
	}
	entriesVec := b.EndVector(len(entries))

	signals.LogBatchStart(b)
	signals.LogBatchAddEntries(b, entriesVec)
	batchOff := signals.LogBatchEnd(b)

	signals.SignalEnvelopeStart(b)
	signals.SignalEnvelopeAddPayloadType(b, signals.SignalPayloadLogBatch)
	signals.SignalEnvelopeAddPayload(b, batchOff)
	envOff := signals.SignalEnvelopeEnd(b)

	b.Finish(envOff)

	return sizePrefixed(b.FinishedBytes()), nil
}

// ---------------------------------------------------------------------------
// Trace stats batch encoding
// ---------------------------------------------------------------------------

// EncodeTraceStatsBatchRing encodes trace stats directly from a ring buffer segment, using a pooled builder.
// EncodeTraceStatsBatchRing encodes trace stats directly from a ring buffer segment.
// Returns the builder with FinishSizePrefixed data. The caller must
// send b.FinishedBytes() then call pool.put(b) to recycle the builder.
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
	signals.SignalEnvelopeAddPayloadType(b, signals.SignalPayloadTraceStatsBatch)
	signals.SignalEnvelopeAddPayload(b, batchOff)
	envOff := signals.SignalEnvelopeEnd(b)

	b.Finish(envOff)

	return b, nil
}

// ---------------------------------------------------------------------------
// Framing helpers
// ---------------------------------------------------------------------------

// sizePrefixed returns a copy of buf with a 4-byte little-endian length prefix.
// This is the framing format expected by the Rust flightrecorder reader.
func sizePrefixed(buf []byte) []byte {
	out := make([]byte, 4+len(buf))
	binary.LittleEndian.PutUint32(out, uint32(len(buf)))
	copy(out[4:], buf)
	return out
}

// SendScatterGather writes a length-prefixed frame using writev (net.Buffers)
// to avoid copying the payload just to prepend 4 bytes.
func SendScatterGather(conn net.Conn, payload []byte) error {
	var prefix [4]byte
	binary.LittleEndian.PutUint32(prefix[:], uint32(len(payload)))
	bufs := net.Buffers{prefix[:], payload}
	_, err := bufs.WriteTo(conn)
	return err
}
