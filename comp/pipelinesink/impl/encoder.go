// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipelinesinkimpl

import (
	"encoding/binary"

	flatbuffers "github.com/google/flatbuffers/go"

	signals "github.com/DataDog/datadog-agent/schema/pipelinesink/generated/signals"
)

// capturedMetric is an internal copy of a metric sample, safe to retain across hook callbacks.
type capturedMetric struct {
	Name         string
	Value        float64
	Tags         []string
	TagPoolSlice *[]string // non-nil when Tags was borrowed from tagPool
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

// builderPool could be used to recycle builders, but for now we create one per
// encode call — the Builder.Reset() reuse pattern is the main win over capnp.

// EncodeMetricBatch serialises samples into a size-prefixed FlatBuffers SignalEnvelope(MetricBatch).
func EncodeMetricBatch(samples []capturedMetric) ([]byte, error) {
	b := flatbuffers.NewBuilder(1024)

	// Shared scratch slice for tag offsets — avoids per-sample allocation.
	var tagBuf []flatbuffers.UOffsetT

	// Build samples bottom-up: strings first, then tables, then vectors.
	sampleOffsets := make([]flatbuffers.UOffsetT, len(samples))
	for i := len(samples) - 1; i >= 0; i-- {
		s := &samples[i]
		nameOff := b.CreateString(s.Name)
		sourceOff := b.CreateString(s.Source)

		// Tags vector: reuse tagBuf across iterations.
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
		tagsVec := b.EndVector(len(s.Tags))

		signals.MetricSampleStart(b)
		signals.MetricSampleAddName(b, nameOff)
		signals.MetricSampleAddValue(b, s.Value)
		signals.MetricSampleAddTags(b, tagsVec)
		signals.MetricSampleAddTimestampNs(b, s.TimestampNs)
		signals.MetricSampleAddSampleRate(b, s.SampleRate)
		signals.MetricSampleAddSource(b, sourceOff)
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

// EncodeLogBatch serialises entries into a size-prefixed FlatBuffers SignalEnvelope(LogBatch).
func EncodeLogBatch(entries []capturedLog) ([]byte, error) {
	b := flatbuffers.NewBuilder(1024)

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

// sizePrefixed returns a copy of buf with a 4-byte little-endian length prefix.
// This is the framing format expected by the Rust pipelinerecorder reader.
func sizePrefixed(buf []byte) []byte {
	out := make([]byte, 4+len(buf))
	binary.LittleEndian.PutUint32(out, uint32(len(buf)))
	copy(out[4:], buf)
	return out
}
