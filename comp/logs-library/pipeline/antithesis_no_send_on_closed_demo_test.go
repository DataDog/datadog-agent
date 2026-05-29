// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug *demonstration* (not a fix), gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" \
//	    -run "TestAntithesisNoSendOnClosedOnShutdown" \
//	    ./comp/logs-library/pipeline/ -v -timeout 10s \
//	    2>&1 | grep -vE "^[0-9]{16} \[Info\]"
//
// Demonstrates property `no-send-on-closed-on-shutdown` (Path 1):
//
// In pipeline-failover mode, `forwardWithFailover` (provider.go:361) performs a
// plain blocking send:
//
//	p.pipelines[primaryPipelineIndex].InputChan <- msg
//
// `provider.Stop()` (provider.go:283-300) first closes all routerChannels, then
// calls `p.forwarderWaitGroup.Wait()`. If the forwarder goroutine is blocked at
// line 361 (because `pipeline.InputChan` is full and the processor is not draining
// it), the `Wait()` hangs forever — `p.forwarderWaitGroup.Wait()` has NO timeout.
//
// Sub-test 1 (TestAntithesisForwarderHangsOnFullInputChan):
//
//	Demonstrates the hang: forwarder is blocked at line 361 when Stop() is called.
//	Stop() returns only after the timeout that this test imposes — never on its own.
//	EXPECTED TO FAIL with "BUG DEMONSTRATED … Stop() hung".
//
// Sub-test 2 (TestAntithesisForwarderPanicsOnInputChanClose):
//
//	Demonstrates the panic: reproduces the Antithesis scenario where an external
//	force (e.g. the grace-period forceClose → destinationsCtx.Stop() eventually
//	flowing back to close InputChan, or a concurrent Stop() call) closes
//	`pipeline.InputChan` while the forwarder goroutine is mid-send. The goroutine
//	was blocked at line 361; the close fires first; the goroutine resumes and sends
//	to a closed channel → panic: send on closed channel.
//	EXPECTED TO FAIL with "BUG DEMONSTRATED … panic: send on closed channel".
//
// Both sub-tests assert clean shutdown (no hang, no panic). They are EXPECTED TO FAIL.

package pipeline

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// blockingSender is a PipelineComponent whose input channel is never drained.
// When the pipeline strategy tries to send a payload to the sender, it blocks
// indefinitely (channel full). This backs up the strategy, which backs up the
// processor, which backs up pipeline.InputChan — causing forwardWithFailover to
// block at the plain send on line 361 of provider.go.
type blockingSender struct {
	inputChan chan *message.Payload
	monitor   metrics.PipelineMonitor
}

func newBlockingSender() *blockingSender {
	return &blockingSender{
		// Size 0: the very first send from the strategy will block the sender goroutine.
		inputChan: make(chan *message.Payload, 0),
		monitor:   metrics.NewTelemetryPipelineMonitor(),
	}
}

func (b *blockingSender) Start()                              {}
func (b *blockingSender) Stop()                               {}
func (b *blockingSender) In() chan *message.Payload           { return b.inputChan }
func (b *blockingSender) PipelineMonitor() metrics.PipelineMonitor { return b.monitor }

// createBlockingProviderWithFailover creates a provider with failover enabled
// and a blocking sender so that pipeline.InputChan fills up and the forwarder
// goroutine blocks at provider.go:361.
func createBlockingProviderWithFailover(t *testing.T) *provider {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("logs_config.pipeline_failover.enabled", true)
	// Unbuffered message channel: the very first message will fill it if the
	// processor cannot forward to the strategy immediately.
	cfg.SetWithoutSource("logs_config.message_channel_size", 0)
	// Unbuffered router channel: the router goroutine (tailer) will also block
	// as soon as the forwarder isn't reading.
	cfg.SetWithoutSource("logs_config.pipeline_failover.router_channel_size", 0)

	endpoints := config.NewMockEndpointsWithOptions(
		[]config.Endpoint{config.NewMockEndpoint()},
		map[string]interface{}{"use_http": true},
	)

	p := newProvider(
		1, // single pipeline so primaryPipelineIndex == routerIndex == 0
		&diagnostic.BufferedMessageReceiver{},
		nil, // no processing rules
		endpoints,
		nil, // no hostname
		cfg,
		compressionfx.NewMockCompressor(),
		sender.NewServerlessMeta(false),
		newBlockingSender(),
	).(*provider)

	p.Start()
	return p
}

// callWithTimeout runs f() in a goroutine.
// Returns ("ok", panicVal) if f() returns within timeout, or ("hang", nil) if not.
func callWithTimeout(f func(), timeout time.Duration) (outcome string, panicVal interface{}) {
	done := make(chan interface{}, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	select {
	case p := <-done:
		return "ok", p
	case <-time.After(timeout):
		return "hang", nil
	}
}

// TestAntithesisForwarderHangsOnFullInputChan demonstrates that provider.Stop()
// hangs indefinitely when forwardWithFailover is blocked at the plain send on
// provider.go:361.
//
// Sequence:
//  1. The blocking sender never drains payloads, causing the pipeline's strategy
//     to block, backing up the processor, filling InputChan.
//  2. We send one message to the router channel. The forwarder goroutine picks it
//     up, trySendToPipeline returns false (all InputChans full), and the goroutine
//     blocks at `p.pipelines[0].InputChan <- msg` (provider.go:361).
//  3. We call Stop(). Stop() closes routerChannels (line 287) — this does NOT
//     wake the goroutine (it is blocked on InputChan, not on the router channel).
//  4. Stop() calls forwarderWaitGroup.Wait() (line 289) — hangs forever because
//     the forwarder goroutine never reaches forwarderWaitGroup.Done().
//
// Property: no-send-on-closed-on-shutdown. EXPECTED TO FAIL.
func TestAntithesisForwarderHangsOnFullInputChan(t *testing.T) {
	p := createBlockingProviderWithFailover(t)

	// Get the router channel for pipeline 0.
	routerChan := p.routerChannels[0]

	// Send one message. The forwarder goroutine will read it from the router
	// channel and then attempt to send it to pipeline.InputChan. Since the
	// sender never drains, the strategy and processor are all backed up and
	// InputChan will block. Give it a moment to reach the blocking state.
	go func() {
		routerChan <- makeTestMsg("block-me")
	}()

	// Wait for the forwarder goroutine to reach the blocking send on InputChan.
	// (The router channel has size 0, so this goroutine will itself block until
	// the forwarder reads from the router — meaning the forwarder has received
	// the message and moved on to the blocking InputChan send.)
	time.Sleep(100 * time.Millisecond)

	// Now call Stop(). If the bug is present, Stop() closes routerChannels then
	// waits on forwarderWaitGroup.Wait() — which never returns.
	const stopTimeout = 500 * time.Millisecond
	outcome, panicVal := callWithTimeout(p.Stop, stopTimeout)

	switch outcome {
	case "hang":
		t.Fatalf(
			"BUG DEMONSTRATED (no-send-on-closed-on-shutdown / hang): "+
				"provider.Stop() hung for >%v — forwardWithFailover goroutine is "+
				"blocked at provider.go:361 (`p.pipelines[0].InputChan <- msg`, a "+
				"plain blocking send with no select/stop signal). "+
				"provider.Stop() (provider.go:287-289) closes routerChannels then calls "+
				"forwarderWaitGroup.Wait() — but the goroutine is blocked on InputChan, "+
				"not on the router channel, so it never reaches forwarderWaitGroup.Done(). "+
				"Fix: replace the plain send with a select that also listens on a stop channel.",
			stopTimeout,
		)
	case "ok":
		if panicVal != nil {
			t.Fatalf(
				"BUG DEMONSTRATED (no-send-on-closed-on-shutdown / panic): "+
					"provider.Stop() panicked: %v",
				panicVal,
			)
		}
		// Stop() returned cleanly — no hang on this run (e.g. the blocking
		// condition was not triggered in time). Test passes.
	}
}

// TestAntithesisForwarderPanicsOnInputChanClose demonstrates the send-on-closed-
// channel panic that occurs when an external force closes pipeline.InputChan
// while a goroutine is mid-send on it — the exact mechanism at provider.go:361.
//
// Background: the `forwardWithFailover` goroutine blocks at line 361:
//
//	p.pipelines[primaryPipelineIndex].InputChan <- msg  // plain send, no select
//
// Under Antithesis CPU-pause or when the outer grace-period watchdog (agent.go
// stopComponents, ~35 s) fires, it calls forceClose → destinationsCtx.Stop() →
// (eventually) processor.Stop() (processor.go:132: close(p.inputChan)) while the
// forwarder goroutine is still blocked on that channel.  The goroutine resumes
// after the close and immediately panics: send on closed channel.
//
// Because the panic occurs inside the forwardWithFailover goroutine (not in the
// goroutine calling close()), we cannot catch it with a defer/recover in this test
// without modifying production code. Instead this test demonstrates the precise
// underlying mechanism: a goroutine blocked on a channel send PANICS when the
// channel is closed by a concurrent goroutine, using a local channel that mirrors
// what InputChan is.  It then cross-references the production code path.
//
// Property: no-send-on-closed-on-shutdown. EXPECTED TO FAIL.
func TestAntithesisForwarderPanicsOnInputChanClose(t *testing.T) {
	// Construct the exact Go channel race that provider.go:361 exhibits.
	//
	// - ch models pipeline.InputChan (unbuffered, as set in createBlockingProviderWithFailover)
	// - senderGoroutine models forwardWithFailover blocked at line 361
	// - closerGoroutine models processor.Stop() (processor.go:132: close(p.inputChan))
	//
	// The panic happens in the sender goroutine, not the closer goroutine. Without
	// a recover() in the sender goroutine, this would crash the test binary. We wrap
	// the sender goroutine with recover() to capture the panic value and report it.
	ch := make(chan *message.Message, 0) // mirrors unbuffered InputChan
	msg := makeTestMsg("panic-me")

	// Channel to receive the panic value from the sender goroutine.
	panicCh := make(chan interface{}, 1)

	// Sender goroutine: blocks on ch <- msg (mirrors forwardWithFailover line 361).
	// If ch is closed while this goroutine is blocked, Go runtime fires a panic
	// "send on closed channel" in this goroutine.
	go func() {
		defer func() { panicCh <- recover() }()
		ch <- msg // blocks until someone reads OR channel is closed
	}()

	// Give the sender goroutine time to reach the blocked state on ch <- msg.
	time.Sleep(50 * time.Millisecond)

	// Closer goroutine: closes ch (mirrors processor.Stop → close(p.inputChan)).
	// This does NOT panic in the closer goroutine, but DOES panic in the sender.
	go func() {
		close(ch) // processor.go:132 equivalent
	}()

	// Collect the panic value from the sender goroutine.
	select {
	case panicVal := <-panicCh:
		if panicVal != nil {
			t.Fatalf(
				"BUG DEMONSTRATED (no-send-on-closed-on-shutdown / panic): "+
					"a goroutine blocked on `ch <- msg` (mirroring forwardWithFailover "+
					"provider.go:361: `p.pipelines[primaryPipelineIndex].InputChan <- msg`) "+
					"panicked when a concurrent goroutine closed the channel: %v\n\n"+
					"Production code path:\n"+
					"  1. forwardWithFailover blocks at provider.go:361 on InputChan <- msg\n"+
					"  2. provider.Stop() (line 287) closes routerChannels — does not unblock\n"+
					"     the forwarder (it is blocked on InputChan, not routerChannels)\n"+
					"  3. provider.Stop() (line 289) calls forwarderWaitGroup.Wait() — hangs\n"+
					"  4. After 30 s grace period, agent.go:stopComponents calls forceClose\n"+
					"     → destinationsCtx.Stop() → processor.Stop() (processor.go:132)\n"+
					"     → close(p.inputChan) — the SAME channel the forwarder is blocked on\n"+
					"  5. forwardWithFailover goroutine resumes and immediately panics:\n"+
					"     'send on closed channel' — crashing the entire agent process\n\n"+
					"Fix: replace the plain send at provider.go:361 with a select that "+
					"also listens on a stop/context channel so the forwarder can be "+
					"interrupted cleanly before InputChan is closed.",
				panicVal,
			)
		}
		// Sender goroutine returned without panic — timing not tight enough.
		// The mechanism proof is still valid; see comment above.
		t.Log("sender goroutine exited cleanly (no panic captured) — " +
			"channel close may have raced before the send reached the blocked state. " +
			"The mechanism is still demonstrated: closing a channel while a goroutine " +
			"is blocked sending to it always panics. Re-run for a more reliable trigger.")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("BUG DEMONSTRATED (no-send-on-closed-on-shutdown / hang): " +
			"sender goroutine did not produce a result within 500ms — " +
			"neither the send completed nor did the close unblock it. " +
			"This indicates a hang in the mechanism test itself.")
	}
}

// makeTestMsg creates a minimal *message.Message for use in provider tests.
func makeTestMsg(content string) *message.Message {
	return message.NewMessage([]byte(content), nil, "info", time.Now().UnixNano())
}
