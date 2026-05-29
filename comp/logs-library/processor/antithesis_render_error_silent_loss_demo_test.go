// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug *demonstration* (not a fix), gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" \
//	    -run TestAntithesisProcessorRenderErrorSilentLoss \
//	    ./comp/logs-library/processor/ -v -timeout 10s \
//	    2>&1 | grep -vE "^[0-9]{16} \[Info\]"
//
// Demonstrates property `processor-render-error-no-silent-loss`:
//
// In processor.processMessage() (processor.go:197-215) there are two early-return
// paths that drop a message silently (no counter, no metric):
//
//  1. msg.Render() returns an error (processor.go:198-200): early return before
//     outputChan write.  Triggerable by passing a StateEncoded message — Render()
//     returns ("render call on an encoded message") for StateEncoded.
//
//  2. p.encoder.Encode() returns an error (processor.go:212-214): early return
//     before outputChan write.  Triggerable by a failing Encoder implementation.
//
// In both cases:
//   - p.outputChan is never written
//   - the auditor never receives an ack
//   - the source offset is never advanced (at-least-once preserved, but silently)
//   - no counter or metric signals the drop
//
// The structural guarantee (offset not advanced) is correct and GOOD; the
// observability gap (no metric) is the bug. This test demonstrates both paths
// by asserting the message appears in outputChan after processMessage — it FAILS
// because the message is dropped at the log.Error site.
//
// EXPECTED TO FAIL on both sub-tests.

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	agentconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// demoFailingEncoder always returns an error from Encode(), triggering the
// second silent-drop path in processor.processMessage() (processor.go:212-214).
type demoFailingEncoder struct{}

func (e *demoFailingEncoder) Encode(_ *message.Message, _ string) error {
	return errors.New("synthetic encoder error — demo")
}

// makeDemoProcessor builds a minimal Processor suitable for unit testing.
// inputChan and outputChan are exposed so tests can send/receive messages.
func makeDemoProcessor(encoder Encoder) (*Processor, chan *message.Message, chan *message.Message) {
	inputChan := make(chan *message.Message, 10)
	outputChan := make(chan *message.Message, 10)

	p := &Processor{
		inputChan:                 inputChan,
		outputChan:                outputChan,
		processingRules:           nil, // no redaction rules
		encoder:                   encoder,
		done:                      make(chan struct{}, 1),
		diagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		hostname:                  nil, // GetHostname returns "unknown" when nil
		pipelineMonitor:           metrics.NewNoopPipelineMonitor("antithesis-demo"),
		utilization:               metrics.NewNoopPipelineMonitor("antithesis-demo").MakeUtilizationMonitor("antithesis-demo", "antithesis-demo"),
		instanceID:                "antithesis-demo",
		configChan:                make(chan failoverConfig, 1),
	}
	return p, inputChan, outputChan
}

// makeDemoMsg creates a minimal unstructured message with an Origin so that
// applyRedactingRules passes it through (no processing rules configured).
func makeDemoMsg(content string) *message.Message {
	// Config must be non-nil: applyRedactingRules (processor.go:253) dereferences
	// msg.Origin.LogSource.Config.ProcessingRules unconditionally.
	src := sources.NewLogSource("demo", &agentconfig.LogsConfig{})
	return message.NewMessageWithSource([]byte(content), "info", src, 0)
}

// TestAntithesisProcessorRenderErrorSilentLoss demonstrates Path 1:
// a message in StateEncoded is passed to processMessage(). msg.Render()
// (message.go:379) returns "render call on an encoded message" — processor
// logs log.Error and returns without writing to outputChan.
//
// Property: processor-render-error-no-silent-loss. EXPECTED TO FAIL.
func TestAntithesisProcessorRenderErrorSilentLoss(t *testing.T) {
	p, _, outputChan := makeDemoProcessor(JSONEncoder) // real encoder; Render error fires first

	// Craft a message that is already in StateEncoded.
	// Render() (message.go:378-380) returns an error for StateEncoded:
	//   case StateEncoded:
	//       return m.content, errors.New("render call on an encoded message")
	//
	// This is the exact trigger for processor.go:198-200:
	//   rendered, err := msg.Render()
	//   if err != nil {
	//       log.Error("can't render the msg", err)
	//       return   // <-- outputChan never written
	//   }
	msg := makeDemoMsg("should reach output")
	msg.SetEncoded([]byte(`{"message":"already encoded"}`)) // transitions to StateEncoded

	// Call processMessage directly (same as the run() goroutine).
	p.processMessage(msg)

	// THE PROPERTY UNDER TEST: the message must appear in outputChan.
	// If the channel is empty the message was silently dropped.
	select {
	case out := <-outputChan:
		t.Logf("NOT A BUG: message reached outputChan (content len=%d)", len(out.GetContent()))
	default:
		t.Fatalf(
			"BUG DEMONSTRATED (processor-render-error-no-silent-loss / Render path): "+
				"a message in StateEncoded was passed to processMessage(). "+
				"msg.Render() (message.go:378-380) returned an error "+
				"(\"render call on an encoded message\"). "+
				"processor.processMessage() (processor.go:198-200) logged log.Error "+
				"and returned — outputChan was NOT written. "+
				"The auditor will not advance the source offset for this message; "+
				"no counter or metric records the drop. "+
				"The at-least-once guarantee is preserved (offset not advanced), "+
				"but the drop is invisible from metrics.",
		)
	}
}

// TestAntithesisProcessorEncodeErrorSilentLoss demonstrates Path 2:
// p.encoder.Encode() returns an error; processor drops the message without
// writing to outputChan and without incrementing any counter.
//
// Property: processor-render-error-no-silent-loss. EXPECTED TO FAIL.
func TestAntithesisProcessorEncodeErrorSilentLoss(t *testing.T) {
	p, _, outputChan := makeDemoProcessor(&demoFailingEncoder{})

	// A normal unstructured message: Render() succeeds (returns content as-is for
	// StateUnstructured), but then demoFailingEncoder.Encode() returns an error,
	// triggering processor.go:212-214:
	//   if err := p.encoder.Encode(msg, p.GetHostname(msg)); err != nil {
	//       log.Error("unable to encode msg ", err)
	//       return   // <-- outputChan never written
	//   }
	msg := makeDemoMsg("important log line to be dropped")

	p.processMessage(msg)

	select {
	case out := <-outputChan:
		t.Logf("NOT A BUG: message reached outputChan (content len=%d)", len(out.GetContent()))
	default:
		t.Fatalf(
			"BUG DEMONSTRATED (processor-render-error-no-silent-loss / Encode path): "+
				"encoder.Encode() returned a synthetic error. "+
				"processor.processMessage() (processor.go:212-214) logged log.Error "+
				"and returned — outputChan was NOT written. "+
				"The auditor will not advance the source offset for this message; "+
				"no counter or metric records the drop. "+
				"Under OOM pressure or a struct with non-serializable fields, "+
				"json.Marshal (json.go:62) can return a real error, making this path "+
				"reachable in production.",
		)
	}
}
