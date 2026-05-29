// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug *demonstration* (not a fix), gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" -run TestAntithesisOversizedBatchDropDemo \
//	    ./comp/logs-library/sender/ -v
//
// Demonstrates property `oversized-line-truncation-safe`: when a single message
// whose content exceeds the batch-level `maxContentSize` is fed to the batch
// strategy, `batch.processMessage` calls `addMessage` twice (once after flushing
// any buffered data) and both calls return `(false, nil)`.  The batch logs a
// warning and increments `tlmDroppedTooLarge`, but NEVER adds the message to
// `MessageBuffer` and NEVER forwards its metadata to `outputChan`.
//
// Consequence for the auditor/tailer: the auditor only advances the file offset
// for messages present in `payload.MessageMetas`.  Because the oversized message
// is not in any payload, its offset is never committed.  On agent restart the
// tailer re-reads the same oversized line from the last committed offset, the
// decoder truncates it again, the batch drops it again — an infinite re-read
// loop with no progress signal.
//
// This test ASSERTS that the oversized message appears in at least one output
// payload's MessageMetas (i.e. its offset is accounted for).  The test FAILS
// because the current implementation silently drops it — BUG DEMONSTRATED.
//
// The normal message sent AFTER the oversized one is used to flush the batch
// (by stopping the strategy) so we can verify the output channel is not silent
// overall; only the oversized message is absent.

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// TestAntithesisOversizedBatchDropDemo feeds a message that exceeds the batch
// content-size limit directly through processMessage and then sends a normal
// message to force a flush.  It then inspects every received payload to check
// whether the oversized message's metadata was carried in any payload.  The
// assertion EXPECTS the oversized message to be accounted for — it will FAIL
// because the current code silently drops it (bug demonstration).
func TestAntithesisOversizedBatchDropDemo(t *testing.T) {
	const (
		maxBatchSize   = 10            // generous count limit
		maxContentSize = 50            // bytes — small enough for the oversized msg to exceed
		pipelineName   = "antithesis-demo"
	)

	// Build the oversized content: 200 bytes, well above maxContentSize=50.
	// In production this would be a truncated log line that still exceeds the
	// batch limit (e.g. lineLimit ≈ batch_max_content_size).
	oversizedContent := []byte(strings.Repeat("X", 200))
	normalContent := []byte("normal")

	// Wire up the strategy the same way production code does.
	inputChan := make(chan *message.Message, 10)
	outputChan := make(chan *message.Payload, 10)
	flushChan := make(chan struct{}, 1)

	strategy := NewBatchStrategy(
		inputChan,
		outputChan,
		flushChan,
		NewMockServerlessMeta(false),
		time.Hour, // long flush interval — we control flushing manually
		maxBatchSize,
		maxContentSize,
		pipelineName,
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"antithesis-demo",
	)
	strategy.Start()

	// Send the oversized message.
	oversizedMsg := message.NewMessage(oversizedContent, nil, "", 0)
	inputChan <- oversizedMsg

	// Send a normal message to give the strategy something to flush.
	normalMsg := message.NewMessage(normalContent, nil, "", 0)
	inputChan <- normalMsg

	// Stop the strategy — this flushes any pending batch and closes stopChan.
	// We must drain outputChan first in a goroutine so Stop() doesn't deadlock.
	done := make(chan struct{})
	var payloads []*message.Payload
	go func() {
		defer close(done)
		strategy.Stop()
	}()

	// Drain all payloads until the strategy goroutine exits (signalled by
	// inputChan being closed, which happens inside Stop()).
	// We collect payloads from outputChan until both the stop goroutine has
	// finished and the channel is drained.
	<-done
	drainLoop:
	for {
		select {
		case p := <-outputChan:
			payloads = append(payloads, p)
		default:
			break drainLoop
		}
	}

	t.Logf("received %d payload(s) from the batch strategy", len(payloads))
	for i, p := range payloads {
		t.Logf("  payload[%d]: %d message meta(s)", i, len(p.MessageMetas))
		for j, meta := range p.MessageMetas {
			t.Logf("    meta[%d]: RawDataLen=%d", j, meta.RawDataLen)
		}
	}

	// Count how many metas have RawDataLen matching each message.
	// Note: MessageBuffer.AddMessage *copies* MessageMetadata, so we cannot use
	// pointer identity.  Instead we match by RawDataLen (set to len(content) by
	// NewMessage) — sufficient because our two messages have different sizes.
	oversizedRawLen := len(oversizedContent) // 200
	normalRawLen := len(normalContent)       // 6

	var oversizedDelivered, normalDelivered int
	for _, p := range payloads {
		for _, meta := range p.MessageMetas {
			switch meta.RawDataLen {
			case oversizedRawLen:
				oversizedDelivered++
			case normalRawLen:
				normalDelivered++
			}
		}
	}

	// The normal message must appear in some payload (sanity check — if this
	// fails the strategy didn't flush at all and the test is invalid).
	if normalDelivered == 0 {
		t.Logf("SANITY FAILURE: normal message (RawDataLen=%d) was not in any payload — strategy may not have flushed", normalRawLen)
	} else {
		t.Logf("OK: normal message appeared in %d payload meta(s) (strategy flushed correctly)", normalDelivered)
	}

	// THE PROPERTY UNDER TEST: the oversized message must also appear in at
	// least one payload's MessageMetas so that the auditor can advance its
	// offset.  If it is absent the tailer will re-read the same line on
	// restart — an infinite loop with silent data loss.
	if oversizedDelivered == 0 {
		t.Fatalf(
			"BUG DEMONSTRATED (oversized-line-truncation-safe): "+
				"the oversized message (content len=%d, batch maxContentSize=%d) "+
				"was silently dropped by the batch strategy. "+
				"Its metadata (RawDataLen=%d) does NOT appear in any of the %d output payload(s). "+
				"The auditor will never advance past this offset; on agent restart "+
				"the tailer will re-read this line indefinitely.",
			len(oversizedContent), maxContentSize, oversizedRawLen, len(payloads),
		)
	}

	t.Logf("NOT A BUG: oversized message metadata was present in %d payload(s) — offset will be advanced", oversizedDelivered)
}
