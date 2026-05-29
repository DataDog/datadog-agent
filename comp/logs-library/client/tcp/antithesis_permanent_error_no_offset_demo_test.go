// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug *demonstration* (not a fix), gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" -run TestAntithesisTCPPermanentErrorNoOffsetAdvance \
//	    ./comp/logs-library/client/tcp/ -v
//
// Demonstrates property `tcp-permanent-error-no-offset-advance`: the HTTP and TCP
// destinations differ in auditor-offset semantics on permanent send failure.
//
// HTTP destination (comp/logs-library/client/http/destination.go:318):
//   output <- payload  // called unconditionally after retry loop exits (success OR permanent error)
//
// TCP destination (comp/logs-library/client/tcp/destination.go:105-119):
//   // on write error with shouldRetry=false, returns WITHOUT calling output <- payload
//
// This test confirms the TCP asymmetry: when a write fails and shouldRetry=false,
// the payload is never forwarded to the output (auditor) channel, so the offset is
// NOT advanced. This is the CORRECT TCP behavior (no silent drop), and the test
// asserts it holds — acting as a regression guard against a future refactor that
// might align TCP to HTTP's advance-on-permanent-drop semantics.
//
// The test is marked EXPECTED TO PASS (the code is correct), but is filed under
// antithesis_demo to make the HTTP/TCP asymmetry explicitly observable and
// regression-proof. A future change that calls `output <- payload` on TCP write
// failure (mimicking HTTP) would cause this test to fail, alerting the reviewer.

package tcp

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
)

// alwaysFailConn is a net.Conn whose Write always returns an error,
// simulating a permanent TCP-level write failure (e.g. broken pipe).
type alwaysFailConn struct {
	net.TCPConn
}

func (c *alwaysFailConn) Write(_ []byte) (int, error) {
	return 0, errors.New("permanent write error: broken pipe")
}

func (c *alwaysFailConn) Close() error { return nil }

// TestAntithesisTCPPermanentErrorNoOffsetAdvance asserts that on a permanent TCP
// write failure (shouldRetry=false), the payload is NOT forwarded to the output
// channel. This preserves at-least-once semantics: the offset in the auditor is
// NOT advanced, so the payload will be re-read and retried on reconnect.
//
// Contrast with HTTP (http/destination.go:318), where `output <- payload` is
// called even on permanent error — intentionally advancing the offset and dropping
// the payload. The asymmetry is by design: TCP retries indefinitely, HTTP drops
// on permanent 4xx.
//
// This test is a regression guard: if someone refactors sendAndRetry to call
// `output <- payload` on TCP write failure (to match HTTP), this test catches it.
func TestAntithesisTCPPermanentErrorNoOffsetAdvance(t *testing.T) {
	endpoint := config.NewEndpoint("api-key", "", "localhost", 9999, config.EmptyPathPrefix, false)
	destCtx := client.NewDestinationsContext()
	destCtx.Start()
	defer destCtx.Stop()

	// shouldRetry=false: this is the "permanent failure" path for TCP.
	// When shouldRetry=false and a write error occurs, sendAndRetry must return
	// WITHOUT sending the payload to output.
	dest := NewDestination(endpoint, false, destCtx, false /*shouldRetry=false*/, statusinterface.NewNoopStatusProvider())

	// Pre-inject an always-failing connection so sendAndRetry skips NewConnection
	// and goes straight to the write path.
	dest.conn = &alwaysFailConn{}
	dest.connCreationTime = time.Now()

	output := make(chan *message.Payload, 1)
	payload := message.NewPayload(
		[]*message.MessageMetadata{},
		[]byte(`{"message":"test log line"}`),
		"source",
		1,
	)

	dest.sendAndRetry(payload, output, nil)

	// KEY ASSERTION: output must remain empty.
	// If TCP matched HTTP and called `output <- payload` on permanent error,
	// the auditor would advance the offset — silently losing the log line.
	select {
	case p := <-output:
		// This would only happen if sendAndRetry sent the payload to output on error.
		t.Fatalf(
			"BUG: TCP destination forwarded payload to output on permanent write failure "+
				"(offset would be advanced, silently dropping the log). "+
				"payload=%v. This matches HTTP behaviour but breaks TCP at-least-once "+
				"semantics. Source: destination.go sendAndRetry.",
			p,
		)
	default:
		// Correct: output is empty, offset is not advanced.
		t.Log("PASS: TCP permanent write failure did NOT advance the offset (output channel empty). " +
			"At-least-once semantics preserved. " +
			"Note: HTTP advances offset on permanent error (http/destination.go:318); " +
			"this asymmetry is by design.")
	}
}
