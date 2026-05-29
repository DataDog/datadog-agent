// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Package pipeline provides log processing pipeline functionality.
//
// failover_ordering_repro_test.go — Antithesis demo: per-source ordering violation
// under pipeline failover.
//
// Property under test: per-source-ordering-preserved
//
//	README:156 states "Each pipeline handles messages in-order, so assigning each
//	input to a single pipeline ensures that input's messages will be delivered to
//	the intake in-order."
//
// Suspected violation (provider.go:353-388):
//
//	With pipeline_failover.enabled=true, trySendToPipeline iterates through all
//	pipelines using non-blocking sends. If the primary pipeline's InputChan is
//	full (backpressured), the message is routed to a secondary pipeline. If a
//	later message from the same source finds the primary pipeline still full, it
//	too gets rerouted. The two pipelines drain independently, so the secondary
//	pipeline may deliver its message BEFORE the primary finishes draining.
//	Result: output order no longer matches submission order for a single source.
//
// Run with:
//
//	go test -tags "antithesis_demo test" \
//	    -run TestFailoverCrossesSourceAcrossPipelines \
//	    ./comp/logs-library/pipeline/ -v
package pipeline

import (
	"fmt"
	"testing"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// newPipelineForRepro builds a Pipeline stub with only InputChan and pipelineMonitor
// populated. The processor and strategy fields are nil — we intentionally never call
// Start()/Stop() on these stubs; we read directly from InputChan in the test.
// NoopPipelineMonitor.GetCapacityMonitor returns nil, but CapacityMonitor.AddIngress
// has a nil guard (capacity_monitor.go:47-50), so no panic occurs.
func newPipelineForRepro(chanSize int) *Pipeline {
	return &Pipeline{
		InputChan:       make(chan *message.Message, chanSize),
		pipelineMonitor: metrics.NewNoopPipelineMonitor("repro"),
	}
}

// newSeqMsg returns a message whose raw content is the given label string.
// origin is nil — none of the routing functions inspect message origin.
func newSeqMsg(label string) *message.Message {
	return message.NewMessage([]byte(label), nil, "info", time.Now().UnixNano())
}

// TestFailoverCrossesSourceAcrossPipelines is the deterministic core repro.
//
// Strategy: construct a minimal provider struct (no real sender/processor) with
// failoverEnabled=true and 2 pipelines whose InputChans each hold exactly 1 msg.
// Then:
//
//  1. Call trySendToPipeline(MSG-1, primary=0).
//     Pipeline 0 is empty → non-blocking send succeeds → MSG-1 in p0 (p0 now full).
//
//  2. Call trySendToPipeline(MSG-2, primary=0).
//     Pipeline 0 is full → non-blocking send fails → tries pipeline 1 (empty) →
//     MSG-2 in p1. Cross-pipeline assignment confirmed.
//
//  3. Drain pipeline 1 first (fast pipeline), then pipeline 0 (slow/stalled).
//     Output order: [MSG-2, MSG-1] — reversed from submission order [MSG-1, MSG-2].
//
// The test FAILS with a clear diagnostic when reordering is detected.
// In the current implementation this ALWAYS fails deterministically.
func TestFailoverCrossesSourceAcrossPipelines(t *testing.T) {
	// InputChan size = 1: MSG-1 fills p0 completely, forcing MSG-2 to p1.
	const chanSize = 1

	p0 := newPipelineForRepro(chanSize)
	p1 := newPipelineForRepro(chanSize)

	// Construct a provider directly — no real sender, processor, or goroutines.
	prov := &provider{
		pipelines:            []*Pipeline{p0, p1},
		currentPipelineIndex: atomic.NewUint32(0),
		currentRouterIndex:   atomic.NewUint32(0),
		failoverEnabled:      true,
	}

	msg1 := newSeqMsg("MSG-1")
	msg2 := newSeqMsg("MSG-2")

	// ── Step 1: route MSG-1 ──────────────────────────────────────────────────
	// Pipeline 0 is empty → attempt 0 succeeds → MSG-1 lands in p0.
	if !prov.trySendToPipeline(msg1, 0) {
		t.Fatal("trySendToPipeline(MSG-1): expected success on empty pipeline 0")
	}
	if len(p0.InputChan) != 1 || len(p1.InputChan) != 0 {
		t.Fatalf("after MSG-1: p0=%d (want 1), p1=%d (want 0)",
			len(p0.InputChan), len(p1.InputChan))
	}
	t.Logf("After MSG-1: p0.InputChan=%d/1, p1.InputChan=%d/1",
		len(p0.InputChan), len(p1.InputChan))

	// ── Step 2: route MSG-2 ──────────────────────────────────────────────────
	// Pipeline 0 is now FULL (capacity=1).
	// trySendToPipeline iterates:
	//   attempt 0 → p0 full → default branch (continue)        [provider.go:383]
	//   attempt 1 → p1 empty → succeeds → MSG-2 in p1.         [provider.go:378]
	if !prov.trySendToPipeline(msg2, 0) {
		t.Fatal("trySendToPipeline(MSG-2): expected failover success on pipeline 1")
	}
	if len(p0.InputChan) != 1 || len(p1.InputChan) != 1 {
		t.Fatalf("after MSG-2: p0=%d (want 1), p1=%d (want 1)",
			len(p0.InputChan), len(p1.InputChan))
	}
	t.Logf("After MSG-2: p0.InputChan=%d/1 (holds MSG-1), p1.InputChan=%d/1 (holds MSG-2)",
		len(p0.InputChan), len(p1.InputChan))
	t.Logf("Cross-pipeline split confirmed: MSG-1→pipeline-0, MSG-2→pipeline-1")

	// ── Step 3: differential drain ───────────────────────────────────────────
	// Simulate pipeline 1 draining faster than pipeline 0 — the real-world
	// trigger is network backpressure on p0's destination while p1 flows freely.
	var got []string

	select {
	case m := <-p1.InputChan: // fast pipeline drains first
		got = append(got, string(m.GetContent()))
	default:
		t.Fatal("pipeline 1 unexpectedly empty")
	}
	select {
	case m := <-p0.InputChan: // slow pipeline drains second
		got = append(got, string(m.GetContent()))
	default:
		t.Fatal("pipeline 0 unexpectedly empty")
	}

	t.Logf("Submission order : [MSG-1 MSG-2]")
	t.Logf("Delivery order   : %v", got)

	// README:156 requires delivery order == submission order for a single source.
	if got[0] != "MSG-1" {
		t.Fatalf(
			"BUG REPRODUCED — per-source-ordering-preserved violated:\n"+
				"  README:156: \"assigning each input to a single pipeline ensures that\n"+
				"               input's messages will be delivered to the intake in-order\"\n"+
				"  Submission order : [MSG-1 MSG-2]\n"+
				"  Delivery order   : %v\n"+
				"  Root cause       : trySendToPipeline (provider.go:372-388) routed MSG-1 to\n"+
				"                     pipeline 0 and MSG-2 (under backpressure) to pipeline 1.\n"+
				"                     Pipeline 1 drained before pipeline 0, reversing the order.",
			got,
		)
	}

	// This line is unreachable in the current implementation because the reorder
	// is structurally guaranteed when pipeline 0 fills before pipeline 1.
	t.Logf("Ordering preserved: %v", got)
}

// TestFailoverCrossesSourceAcrossPipelinesViaForwarder exercises the production
// forwardWithFailover goroutine (provider.go:353-366) rather than calling
// trySendToPipeline directly.
//
// Setup:
//   - 2 pipelines, InputChan capacity = 1 each.
//   - 1 router channel fed by a single logical source (routerIndex=0).
//   - No consumer on pipeline 0 after the first message (fills immediately).
//   - Pipeline 1 is drained after routing completes (fast drain).
//
// MSG-0 lands in p0 (capacity filled). MSG-1 overflows to p1.
// Draining p1 first then p0 produces [MSG-1, MSG-0] — reversed.
func TestFailoverCrossesSourceAcrossPipelinesViaForwarder(t *testing.T) {
	const pipelineChanSize = 1
	const routerChanSize = 8

	p0 := newPipelineForRepro(pipelineChanSize)
	p1 := newPipelineForRepro(pipelineChanSize)

	routerCh := make(chan *message.Message, routerChanSize)

	prov := &provider{
		pipelines:            []*Pipeline{p0, p1},
		currentPipelineIndex: atomic.NewUint32(0),
		currentRouterIndex:   atomic.NewUint32(0),
		failoverEnabled:      true,
		routerChannels:       []chan *message.Message{routerCh},
	}
	prov.forwarderWaitGroup.Add(1)
	go prov.forwardWithFailover(0) // routerIndex=0 → primary pipeline=0

	// Send exactly 2 messages sequentially labelled MSG-0 and MSG-1.
	// Both originate from the same logical source (same router channel).
	for i := 0; i < 2; i++ {
		label := fmt.Sprintf("MSG-%d", i)
		select {
		case routerCh <- newSeqMsg(label):
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout sending %s to router channel", label)
		}
	}

	// Give forwarder goroutine time to dispatch both messages.
	time.Sleep(30 * time.Millisecond)

	t.Logf("After routing: p0.InputChan=%d/1, p1.InputChan=%d/1",
		len(p0.InputChan), len(p1.InputChan))

	// Differential drain: pipeline 1 first (fast), pipeline 0 second (slow).
	// This models the real scenario: p1 unblocked, p0 backpressured.
	var output []string

	for {
		select {
		case m := <-p1.InputChan:
			output = append(output, string(m.GetContent()))
			continue
		default:
		}
		break
	}
	for {
		select {
		case m := <-p0.InputChan:
			output = append(output, string(m.GetContent()))
			continue
		default:
		}
		break
	}

	// Shut down the forwarder.
	close(routerCh)
	prov.forwarderWaitGroup.Wait()

	t.Logf("Submission order : [MSG-0 MSG-1]")
	t.Logf("Delivery order   : %v", output)

	if len(output) < 2 {
		t.Fatalf("expected 2 messages routed; got %d — timing issue or routing failed", len(output))
	}

	// Assert submission order == delivery order.
	for i := 1; i < len(output); i++ {
		if output[i] < output[i-1] {
			t.Fatalf(
				"BUG REPRODUCED (forwarder path) — per-source-ordering-preserved violated:\n"+
					"  README:156: \"assigning each input to a single pipeline ensures that\n"+
					"               input's messages will be delivered to the intake in-order\"\n"+
					"  Submission order : [MSG-0 MSG-1]\n"+
					"  Delivery order   : %v\n"+
					"  Root cause       : forwardWithFailover (provider.go:353-366) split consecutive\n"+
					"                     messages from the same source across different pipeline\n"+
					"                     InputChans; pipelines drained at different rates.\n"+
					"  Cross-pipeline   : MSG-0→pipeline-0 (stalled), MSG-1→pipeline-1 (fast).\n"+
					"                     Pipeline-1 flushed before pipeline-0 → reversal.",
				output,
			)
		}
	}

	t.Logf("No reorder observed in this run. " +
		"Cross-pipeline routing may not have been triggered. " +
		"Run TestFailoverCrossesSourceAcrossPipelines for the deterministic repro.")
}

// TestTrySendToPipelineRoutesToDifferentPipeline directly documents
// the provider.go:372-388 routing behaviour: when the primary pipeline's
// InputChan is full, consecutive messages from the same source are placed in
// different pipelines.  This test PASSES — it is documentation, not a failure
// assertion.  The ordering consequence is shown in
// TestFailoverCrossesSourceAcrossPipelines.
func TestTrySendToPipelineRoutesToDifferentPipeline(t *testing.T) {
	p0 := newPipelineForRepro(1) // capacity 1
	p1 := newPipelineForRepro(1) // capacity 1

	prov := &provider{
		pipelines:            []*Pipeline{p0, p1},
		currentPipelineIndex: atomic.NewUint32(0),
		currentRouterIndex:   atomic.NewUint32(0),
		failoverEnabled:      true,
	}

	// Send MSG-A: p0 empty → lands in p0 (p0 now full).
	msgA := newSeqMsg("MSG-A")
	if !prov.trySendToPipeline(msgA, 0) {
		t.Fatal("could not send MSG-A")
	}
	if len(p0.InputChan) != 1 {
		t.Fatalf("expected MSG-A in p0; p0.InputChan=%d", len(p0.InputChan))
	}

	// Send MSG-B: p0 is full → overflows to p1.
	msgB := newSeqMsg("MSG-B")
	if !prov.trySendToPipeline(msgB, 0) {
		t.Fatal("could not send MSG-B")
	}
	if len(p1.InputChan) != 1 {
		t.Fatalf("expected MSG-B in p1; p1.InputChan=%d", len(p1.InputChan))
	}

	gotA := string((<-p0.InputChan).GetContent())
	gotB := string((<-p1.InputChan).GetContent())

	t.Logf("MSG-A → pipeline 0: content=%q", gotA)
	t.Logf("MSG-B → pipeline 1: content=%q", gotB)

	if gotA != "MSG-A" || gotB != "MSG-B" {
		t.Fatalf("unexpected content: p0=%q (want MSG-A), p1=%q (want MSG-B)", gotA, gotB)
	}

	t.Logf("DOCUMENTED: consecutive messages from one source split across pipelines 0 and 1. " +
		"If pipeline 1 drains before pipeline 0, delivery order is [MSG-B, MSG-A] — reversed.")
}
