// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug *demonstration* (not a fix), gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" \
//	    -run TestAntithesisBatchEncodeFailureSilentDrop \
//	    ./comp/logs-library/sender/ -v -timeout 10s \
//	    2>&1 | grep -vE "^[0-9]{16} \[Info\]"
//
// Demonstrates property `batch-encode-failure-no-silent-batch-loss`:
//
// When the serializer returns an error from Serialize(), batch.processMessage()
// (batch.go:94-98) calls resetBatch() and returns — no payload is ever written to
// outputChan, no metric is incremented, and the message's offset is never advanced.
// The drop is observable only via a log.Warn line.
//
// This test injects a failing serializer (errorSerializer, also defined in
// batch_test.go for the production test suite) directly into the batch struct,
// then asserts that the message metadata appears in outputChan. The test FAILS
// because the message is silently dropped — BUG DEMONSTRATED.
//
// Note: the errorSerializer type is also defined in batch_test.go (same package,
// `test` build tag); we redefine it here under `antithesis_demo` to be self-contained.

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// demoErrorSerializer always returns an error from Serialize() and Finish(),
// triggering the silent-drop path in batch.processMessage() (batch.go:96-98).
type demoErrorSerializer struct{}

func (e *demoErrorSerializer) Serialize(_ *message.Message, _ io.Writer) error {
	return errors.New("synthetic serialization error — demo")
}
func (e *demoErrorSerializer) Finish(_ io.Writer) error {
	return errors.New("synthetic finish error — demo")
}
func (e *demoErrorSerializer) Reset() {}

// demoPassthroughCompressor is a compression.Compressor that writes bytes
// unchanged, so the test never touches real compression logic.
type demoPassthroughCompressor struct{}

func (c *demoPassthroughCompressor) Compress(src []byte) ([]byte, error)   { return src, nil }
func (c *demoPassthroughCompressor) Decompress(src []byte) ([]byte, error) { return src, nil }
func (c *demoPassthroughCompressor) CompressBound(n int) int               { return n }
func (c *demoPassthroughCompressor) ContentEncoding() string               { return "identity" }
func (c *demoPassthroughCompressor) NewStreamCompressor(buf *bytes.Buffer) compression.StreamCompressor {
	return &demoStreamCompressor{Buffer: buf}
}

type demoStreamCompressor struct{ *bytes.Buffer }

func (s *demoStreamCompressor) Close() error { return nil }
func (s *demoStreamCompressor) Flush() error { return nil }

// makeDemoBatch creates a batch with the passthrough compressor.
// The serializer is replaced below to inject the error.
func makeDemoBatch() *batch {
	comp := &demoPassthroughCompressor{}
	var encodedPayload bytes.Buffer
	compressor := comp.NewStreamCompressor(&encodedPayload)
	wc := newWriterWithCounter(compressor)

	return &batch{
		buffer:          NewMessageBuffer(10, 1024),
		serializer:      NewArraySerializer(), // replaced below
		compression:     comp,
		compressor:      compressor,
		writeCounter:    wc,
		encodedPayload:  &encodedPayload,
		pipelineName:    "antithesis-demo",
		pipelineMonitor: metrics.NewNoopPipelineMonitor(""),
		instanceID:      "antithesis-demo",
		utilization:     metrics.NewNoopPipelineMonitor("").MakeUtilizationMonitor("antithesis-demo", "antithesis-demo"),
		serverlessMeta:  NewMockServerlessMeta(false),
	}
}

// TestAntithesisBatchEncodeFailureSilentDrop demonstrates that when
// serializer.Serialize() returns an error, batch.processMessage() drops the
// message silently: no payload is written to outputChan, no counter is
// incremented, and the message's offset is never committed.
//
// Property: batch-encode-failure-no-silent-batch-loss. EXPECTED TO FAIL.
func TestAntithesisBatchEncodeFailureSilentDrop(t *testing.T) {
	b := makeDemoBatch()

	// Inject the failing serializer: every call to Serialize returns an error.
	// This triggers batch.go:94-98:
	//   added, err := b.addMessage(m)
	//   if err != nil {
	//       log.Warn("Encoding failed - dropping payload", err)
	//       b.resetBatch()
	//       return   // <-- message dropped; outputChan never written
	//   }
	b.serializer = &demoErrorSerializer{}

	outputChan := make(chan *message.Payload, 10)
	msg := message.NewMessage([]byte("important log line"), nil, "info", 0)

	// Feed the message through the batch layer.
	b.processMessage(msg, outputChan)

	// THE PROPERTY UNDER TEST: the message metadata must appear in at least one
	// payload on outputChan so that the auditor can advance the source offset.
	// If the channel is empty, the message has been silently dropped.
	select {
	case payload := <-outputChan:
		// If we reach here, the message survived — no bug on this path.
		t.Logf("NOT A BUG: message appeared in payload with %d meta(s)", len(payload.MessageMetas))
	default:
		t.Fatalf(
			"BUG DEMONSTRATED (batch-encode-failure-no-silent-batch-loss): "+
				"when serializer.Serialize() returns an error, batch.processMessage() "+
				"(batch.go:94-98) calls resetBatch() and returns without writing to "+
				"outputChan. The message metadata is NOT in any payload — the auditor "+
				"will not advance the source offset for this message. "+
				"The only signal is a log.Warn(\"Encoding failed - dropping payload\") at "+
				"batch.go:96; there is no counter, no tlmPayloadsDropped increment, and "+
				"no BytesMissed metric. Under OOM pressure or compressor state corruption "+
				"this drop path becomes reachable and is invisible from metrics.",
		)
	}
}

// TestAntithesisBatchFinishFailureSilentDrop demonstrates the second drop path:
// when serializer.Finish() returns an error in flushBuffer() (batch.go:146-151),
// the already-buffered messages are discarded without writing to outputChan.
//
// Property: batch-encode-failure-no-silent-batch-loss. EXPECTED TO FAIL.
func TestAntithesisBatchFinishFailureSilentDrop(t *testing.T) {
	b := makeDemoBatch()
	outputChan := make(chan *message.Payload, 10)

	// Add a message with the working serializer so it lands in the buffer.
	msg := message.NewMessage([]byte("buffered log line"), nil, "info", 0)
	added, err := b.addMessage(msg)
	if !added || err != nil {
		t.Fatalf("setup: addMessage returned added=%v err=%v; test invalid", added, err)
	}

	// Now swap in the failing serializer so that Finish() fails.
	// This triggers batch.go:146-151 (flushBuffer):
	//   if err := b.serializer.Finish(b.writeCounter); err != nil {
	//       log.Warn("Encoding failed - dropping payload", err)
	//       b.resetBatch()
	//       b.utilization.Stop()
	//       return   // <-- buffered messages dropped; outputChan never written
	//   }
	b.serializer = &demoErrorSerializer{}
	b.flushBuffer(outputChan, "timer")

	select {
	case payload := <-outputChan:
		t.Logf("NOT A BUG: buffered message appeared in payload with %d meta(s)", len(payload.MessageMetas))
	default:
		t.Fatalf(
			"BUG DEMONSTRATED (batch-encode-failure-no-silent-batch-loss / Finish path): "+
				"when serializer.Finish() returns an error during flushBuffer() "+
				"(batch.go:146-151), all messages that were already buffered are discarded. "+
				"outputChan was not written — the auditor never sees these messages. "+
				"There is no counter or metric for this drop, only a log.Warn.",
		)
	}
}
