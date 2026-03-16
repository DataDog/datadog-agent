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

	signals "github.com/DataDog/datadog-agent/schema/flightrecorder/generated/signals"
)

// metricPoint is a compact data point for known contexts (fast path).
// Only 32 bytes — no heap pointers, no strings. This is what 99.9% of
// ring buffer items look like after context warm-up.
type metricPoint struct {
	ContextKey  uint64
	Value       float64
	TimestampNs int64
	SampleRate  float64
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

// capturedLog is an internal copy of a log entry, safe to retain across hook callbacks.
type capturedLog struct {
	Content          []byte
	ContentPoolSlice *[]byte   // non-nil when Content was borrowed from contentPool
	Status           string
	Tags             []string
	TagPoolSlice     *[]string // non-nil when Tags was borrowed from tagPool
	Hostname         string
	TimestampNs      int64
	Source           string
}

// ---------------------------------------------------------------------------
// Builder pool (optimisation 1B)
// ---------------------------------------------------------------------------

// builderPool recycles FlatBuffers builders between flushes. Builder.Reset()
// preserves the grown backing slice so subsequent encodes skip the resize path.
type builderPool struct {
	pool sync.Pool
}

func newBuilderPool() *builderPool {
	return &builderPool{
		pool: sync.Pool{New: func() any { return flatbuffers.NewBuilder(4096) }},
	}
}

func (p *builderPool) get() *flatbuffers.Builder {
	return p.pool.Get().(*flatbuffers.Builder)
}

func (p *builderPool) put(b *flatbuffers.Builder) {
	b.Reset()
	p.pool.Put(b)
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
func EncodeSplitMetricBatch(
	pool *builderPool,
	defs []contextDef, defTail, defCount, defCap int,
	pts []metricPoint, ptTail, ptCount, ptCap int,
) ([]byte, error) {
	total := defCount + ptCount
	b := pool.get()

	var tagBuf []flatbuffers.UOffsetT
	sampleOffsets := make([]flatbuffers.UOffsetT, total)

	// Encode data points (references) first in reverse — they'll appear after
	// definitions in the final vector.
	for ri := ptCount - 1; ri >= 0; ri-- {
		idx := (ptTail + ri) % ptCap
		p := &pts[idx]
		signals.MetricSampleStart(b)
		signals.MetricSampleAddContextKey(b, p.ContextKey)
		signals.MetricSampleAddValue(b, p.Value)
		signals.MetricSampleAddTimestampNs(b, p.TimestampNs)
		signals.MetricSampleAddSampleRate(b, p.SampleRate)
		sampleOffsets[defCount+ri] = signals.MetricSampleEnd(b)
	}

	// Encode context definitions in reverse.
	for ri := defCount - 1; ri >= 0; ri-- {
		idx := (defTail + ri) % defCap
		d := &defs[idx]

		nameOff := b.CreateString(d.Name)
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
		sampleOffsets[ri] = signals.MetricSampleEnd(b)
	}

	signals.MetricBatchStartSamplesVector(b, total)
	for i := len(sampleOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(sampleOffsets[i])
	}
	samplesVec := b.EndVector(total)

	signals.MetricBatchStart(b)
	signals.MetricBatchAddSamples(b, samplesVec)
	batchOff := signals.MetricBatchEnd(b)

	signals.SignalEnvelopeStart(b)
	signals.SignalEnvelopeAddPayloadType(b, signals.SignalPayloadMetricBatch)
	signals.SignalEnvelopeAddPayload(b, batchOff)
	envOff := signals.SignalEnvelopeEnd(b)

	b.Finish(envOff)

	result := sizePrefixed(b.FinishedBytes())
	pool.put(b)
	return result, nil
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
func EncodeLogBatchRing(pool *builderPool, buf []capturedLog, tail, count, capacity int) ([]byte, error) {
	b := pool.get()

	var tagBuf []flatbuffers.UOffsetT
	entryOffsets := make([]flatbuffers.UOffsetT, count)
	for ri := count - 1; ri >= 0; ri-- {
		idx := (tail + ri) % capacity
		e := &buf[idx]

		contentOff := b.CreateByteVector(e.Content)
		statusOff := b.CreateString(e.Status)
		hostnameOff := b.CreateString(e.Hostname)
		sourceOff := b.CreateString(e.Source)

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

	signals.LogBatchStart(b)
	signals.LogBatchAddEntries(b, entriesVec)
	batchOff := signals.LogBatchEnd(b)

	signals.SignalEnvelopeStart(b)
	signals.SignalEnvelopeAddPayloadType(b, signals.SignalPayloadLogBatch)
	signals.SignalEnvelopeAddPayload(b, batchOff)
	envOff := signals.SignalEnvelopeEnd(b)

	b.Finish(envOff)

	result := sizePrefixed(b.FinishedBytes())
	pool.put(b)
	return result, nil
}

func encodeLogBatchWith(b *flatbuffers.Builder, entries []capturedLog) ([]byte, error) {
	var tagBuf []flatbuffers.UOffsetT
	entryOffsets := make([]flatbuffers.UOffsetT, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		e := &entries[i]

		contentOff := b.CreateByteVector(e.Content)
		statusOff := b.CreateString(e.Status)
		hostnameOff := b.CreateString(e.Hostname)
		sourceOff := b.CreateString(e.Source)

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

// lengthPrefix returns a 4-byte little-endian length prefix for scatter-gather writes.
var lengthPrefixBuf [4]byte

// SendScatterGather writes a length-prefixed frame using writev (net.Buffers)
// to avoid copying the payload just to prepend 4 bytes.
func SendScatterGather(conn net.Conn, payload []byte) error {
	var prefix [4]byte
	binary.LittleEndian.PutUint32(prefix[:], uint32(len(payload)))
	bufs := net.Buffers{prefix[:], payload}
	_, err := bufs.WriteTo(conn)
	return err
}
