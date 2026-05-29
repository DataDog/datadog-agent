// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis property verification (NOT a bug demonstration), gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" \
//	    -run TestAntithesisMRFUnreliableDestinationDropBounded \
//	    ./comp/logs-library/sender/ -v -timeout 30s \
//	    2>&1 | grep -vE "^[0-9]{16} \[Info\]"
//
// Investigates property `mrf-unreliable-destination-drop-bounded`:
//
//  1. When the unreliable (secondary/MRF) destination input buffer is full,
//     NonBlockingSend (worker.go:175) silently drops the payload. The property
//     claims the drop must be COUNTED (tlmPayloadsDropped increments) and must
//     NOT affect the reliable (primary) destination's at-least-once delivery.
//
//  2. Code paths exercised:
//     - worker.go:120-148: reliable-destination blocking send loop
//     - worker.go:167-183: unreliable-destination NonBlockingSend with drop counter
//     - destination_sender.go:134-141: NonBlockingSend select/default
//
// The test constructs a worker with:
//   - one reliable destination whose input channel is actively drained (normal delivery)
//   - one unreliable destination whose input channel is NOT drained (buffer fills, drops occur)
//
// It sends N=10 payloads and asserts:
//   (a) the reliable destination received all N payloads (at-least-once preserved)
//   (b) tlmPayloadsDropped{reliable="false"} incremented at least (N - bufferSize) times
//       (drops are counted, not silent)
//
// Expected outcome: NOT A BUG — the property holds. The test PASSES when the code
// is correct, and FAILS if drops are silent (counter not incremented) OR if a full
// unreliable buffer blocks/loses the primary.

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"fmt"
	"testing"
	"time"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// mrfTestDestination is a self-contained test destination for the MRF drop test.
// It starts a goroutine that:
//   - for reliable (drain=true): continuously reads and counts received payloads
//   - for unreliable (drain=false): blocks on a "begin draining" signal so the
//     input channel fills up during the test, causing NonBlockingSend to drop.
//     When signalled (at shutdown), it drains the remaining items and closes stopChan.
//
// stopChan is closed after input is closed and drained, which is required for
// DestinationSender.Stop() to return.
type mrfTestDestination struct {
	isMRFFlag    bool
	drain        bool // if true, input is drained continuously; if false, drains only on shutdown
	beginDrainCh chan struct{} // closed by the test to permit draining for unreliable dest

	received chan *message.Payload // received payloads (written when drain=true)
	stopChan chan struct{}
}

func newMRFTestDestination(isMRF, drain bool, capacity int) *mrfTestDestination {
	return &mrfTestDestination{
		isMRFFlag:    isMRF,
		drain:        drain,
		beginDrainCh: make(chan struct{}),
		received:     make(chan *message.Payload, capacity),
		stopChan:     make(chan struct{}),
	}
}

func (d *mrfTestDestination) IsMRF() bool { return d.isMRFFlag }
func (d *mrfTestDestination) Target() string {
	if d.isMRFFlag {
		return "mrf-test-dest"
	}
	return "primary-test-dest"
}
func (d *mrfTestDestination) Metadata() *client.DestinationMetadata {
	return client.NewNoopDestinationMetadata()
}

func (d *mrfTestDestination) Start(input chan *message.Payload, _ chan *message.Payload, _ chan bool) (stopChan <-chan struct{}) {
	go func() {
		defer close(d.stopChan)
		if !d.drain {
			// Wait for the shutdown signal before draining. This keeps the input
			// buffer full during the test so NonBlockingSend drops payloads.
			<-d.beginDrainCh
		}
		// Drain whatever is in the channel (including remaining items after close).
		for p := range input {
			if d.drain {
				d.received <- p
			}
			_ = p
		}
	}()
	return d.stopChan
}

// TestAntithesisMRFUnreliableDestinationDropBounded verifies the two-part
// mrf-unreliable-destination-drop-bounded property:
//
//  1. At-least-once on primary: even when the unreliable destination buffer is
//     full, the reliable destination still receives every payload.
//
//  2. Observable drops: tlmPayloadsDropped{reliable="false"} is incremented for
//     each payload dropped from the unreliable destination — drops are counted,
//     not silent.
func TestAntithesisMRFUnreliableDestinationDropBounded(t *testing.T) {
	const (
		bufferSize  = 3  // unreliable dest input capacity; fills after 3 payloads
		numPayloads = 10 // total payloads to send; bufferSize will be received, rest dropped
	)

	cfg := configmock.New(t)

	// --- reliable destination setup ---
	//
	// drain=true: the goroutine started by Start() continuously reads from the
	// input channel. This ensures the worker's reliable-destination blocking
	// Send() (worker.go:122-139) always succeeds and never stalls.
	reliableDest := newMRFTestDestination(false, true, numPayloads+1)

	// --- unreliable destination setup ---
	//
	// drain=false: the goroutine started by Start() blocks on beginDrainCh until
	// we signal it at shutdown. During the test the input buffer fills after
	// bufferSize payloads; subsequent NonBlockingSend calls (worker.go:175)
	// return false and tlmPayloadsDropped.Inc("false","0") is called.
	unreliableDest := newMRFTestDestination(false, false, numPayloads+1)

	// Auditor sink: buffered so the worker's outputChan writes never block.
	auditor := &testAuditor{
		output: make(chan *message.Payload, numPayloads+1),
	}

	// DestinationFactory: called once per worker.
	destinationFactory := func(_ string) *client.Destinations {
		return client.NewDestinations(
			[]client.Destination{reliableDest},   // reliable (primary)
			[]client.Destination{unreliableDest}, // unreliable (secondary/MRF stand-in)
		)
	}

	pipelineMonitor := metrics.NewNoopPipelineMonitor("antithesis-mrf-test")

	// Create and start the worker.
	inputChan := make(chan *message.Payload, numPayloads)
	w := newWorker(cfg, inputChan, auditor, destinationFactory, bufferSize, NewMockServerlessMeta(false), pipelineMonitor, "antithesis-mrf")
	w.start()

	// Capture the baseline drop counter BEFORE sending payloads.
	// tlmPayloadsDropped uses DefaultMetric=true, so it lives in the default registry.
	// We read via GetCompatComponent().Gather(true).
	dropsBefore := readDropCounter(t, "false", "0")

	// Send N payloads.
	for i := range numPayloads {
		inputChan <- &message.Payload{
			Encoded:  []byte(fmt.Sprintf("payload-%d", i)),
			Encoding: "identity",
		}
	}

	// Wait for the reliable destination to have received all N payloads (with timeout).
	receivedCount := 0
	deadline := time.After(10 * time.Second)
	for receivedCount < numPayloads {
		select {
		case <-reliableDest.received:
			receivedCount++
		case <-deadline:
			t.Fatalf(
				"VIOLATION (mrf-unreliable-destination-drop-bounded / at-least-once): "+
					"reliable destination only received %d/%d payloads before timeout. "+
					"A full unreliable-destination buffer may be blocking the reliable send path "+
					"(worker.go:120-148), breaking the at-least-once guarantee for the primary destination. "+
					"Code path: worker.go:122-139 (blocking Send) → destination_sender.go:122-128.",
				receivedCount, numPayloads,
			)
		}
	}
	t.Logf("OK (at-least-once): reliable destination received all %d/%d payloads", receivedCount, numPayloads)

	// Signal the unreliable destination to begin draining so that
	// DestinationSender.Stop() (destination_sender.go:72-76) can complete.
	// This must happen BEFORE w.stop(), which calls destSender.Stop() and waits
	// for stopChan — which is only closed after the drain goroutine exits.
	close(unreliableDest.beginDrainCh)

	// Stop the worker cleanly so all goroutines exit before we read the counter.
	w.stop()

	// Read the drop counter AFTER all payloads have been processed.
	dropsAfter := readDropCounter(t, "false", "0")
	dropsObserved := dropsAfter - dropsBefore

	// How many drops should we have seen?
	// The first bufferSize payloads fit in the unreliable dest input; the rest are dropped.
	expectedDrops := float64(numPayloads - bufferSize)
	t.Logf("drop counter before=%v after=%v observed=%v expectedMin=%v",
		dropsBefore, dropsAfter, dropsObserved, expectedDrops)

	if dropsObserved < expectedDrops {
		t.Fatalf(
			"VIOLATION (mrf-unreliable-destination-drop-bounded / observable-drops): "+
				"expected at least %.0f drop(s) to be counted in tlmPayloadsDropped{reliable=\"false\",destination=\"0\"} "+
				"(unreliable buffer size=%d, payloads sent=%d), but only %.0f drop(s) were observed. "+
				"Drops from NonBlockingSend (worker.go:175) are NOT being counted — they are silently lost. "+
				"Code path: worker.go:175 → tlmPayloadsDropped.Inc(\"false\", \"0\") (worker.go:176).",
			expectedDrops, bufferSize, numPayloads, dropsObserved,
		)
	}

	t.Logf("OK (observable-drops): %.0f drop(s) counted in tlmPayloadsDropped{reliable=\"false\"} (>= expected %.0f)",
		dropsObserved, expectedDrops)
	t.Logf("VERDICT: NOT A BUG — property mrf-unreliable-destination-drop-bounded HOLDS. "+
		"Primary (reliable) destination received all %d payloads; "+
		"%.0f unreliable-destination drop(s) are counted in tlmPayloadsDropped.",
		numPayloads, dropsObserved)
}

// readDropCounter reads the current value of tlmPayloadsDropped for the given
// reliable and destination label values from the global prometheus default registry.
// Returns 0.0 if the metric has not been created yet (i.e. no drops have occurred).
func readDropCounter(t *testing.T, reliable, destination string) float64 {
	t.Helper()
	tel := telemetryimpl.GetCompatComponent()
	// tlmPayloadsDropped uses Options{DefaultMetric: true}, so it lives in the
	// defaultRegistry (defaultGather=true).
	families, err := tel.Gather(true)
	if err != nil {
		t.Logf("readDropCounter: Gather error: %v (treating as 0)", err)
		return 0.0
	}
	for _, fam := range families {
		if fam.GetName() != "logs_sender__payloads_dropped" {
			continue
		}
		for _, m := range fam.GetMetric() {
			var relLabel, destLabel string
			for _, lp := range m.GetLabel() {
				switch lp.GetName() {
				case "reliable":
					relLabel = lp.GetValue()
				case "destination":
					destLabel = lp.GetValue()
				}
			}
			if relLabel == reliable && destLabel == destination {
				if c := m.GetCounter(); c != nil {
					return c.GetValue()
				}
			}
		}
	}
	return 0.0
}
