// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug demonstration (not a fix), gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" \
//	    -run TestAntithesisStaleOriginMetadata \
//	    ./comp/logs-library/processor/ -v -timeout 10s \
//	    2>&1 | grep -vE "^[0-9]{16} \[Info\]"
//
// Demonstrates property `log-metadata-not-corrupted`:
//
// # Background — the stale-origin hazard
//
// Both filterMRFMessages (processor.go:236) and the JSON encoder (json.go:67)
// resolve the service name by calling msg.Origin.Service() at processing time:
//
//	// origin.go:154-161
//	func (o *Origin) Service() string {
//	    if o == nil || o.LogSource == nil {
//	        return ""
//	    }
//	    if o.LogSource.Config.Service != "" {
//	        return o.LogSource.Config.Service  // reads live field — NOT a snapshot
//	    }
//	    return o.service
//	}
//
// The LogSource.Config pointer is shared between the message's Origin and the
// live source registry. Under container churn (rapid container create/destroy),
// the AD scheduler replaces or mutates Config fields on the live LogSource while
// messages from the old container are still in-flight in the pipeline channel.
//
// # What this test does
//
//  1. Creates a LogSource with Config.Service = "svc-a" (simulating container A).
//  2. Creates a message whose Origin points at that source.
//  3. Sends the message into the processor's inputChan.
//  4. Before the message is processed, mutates Config.Service to "svc-b"
//     (simulating container churn: the same LogSource object gets a new service
//     name from an updated AD annotation, or the Config pointer is reused for
//     container B).
//  5. Lets the processor run.
//  6. Asserts the delivered message carries service "svc-a" (the value valid at
//     read time). FAILS if it carries "svc-b" (stale/wrong metadata).
//
// Sub-test 2 repeats the same sequence for the MRF allowlist path: a message
// from "billing-service" is in-flight when the source is mutated to
// "payment-service". The message should be MRF-tagged because its service was
// in the allowlist at creation time. It will NOT be tagged because
// filterMRFMessages reads the mutated value.
//
// EXPECTED TO FAIL on both sub-tests.

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	agentconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// makeStaleOriginProcessor builds a minimal Processor with real channels and the
// real JSON encoder. The configChan is pre-seeded with a zero failoverConfig so
// that the processor does not block on the configChan read in run().
func makeStaleOriginProcessor(encoder Encoder) (*Processor, chan *message.Message, chan *message.Message) {
	inputChan := make(chan *message.Message, 10)
	outputChan := make(chan *message.Message, 10)

	p := &Processor{
		inputChan:                 inputChan,
		outputChan:                outputChan,
		processingRules:           nil,
		encoder:                   encoder,
		done:                      make(chan struct{}, 1),
		diagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		hostname:                  nil,
		pipelineMonitor:           metrics.NewNoopPipelineMonitor("antithesis-stale-origin"),
		utilization:               metrics.NewNoopPipelineMonitor("antithesis-stale-origin").MakeUtilizationMonitor("antithesis-stale-origin", "antithesis-stale-origin"),
		instanceID:                "antithesis-stale-origin",
		configChan:                make(chan failoverConfig, 1),
	}
	// Pre-seed configChan so run() picks up the initial failoverConfig immediately.
	p.configChan <- failoverConfig{}
	return p, inputChan, outputChan
}

// serviceFromEncoded decodes the JSON-encoded message payload and returns the
// "service" field. The processor's JSON encoder writes the final wire payload
// (json.go:62-76); the delivered message is in StateEncoded with its content set
// to the encoded bytes.
func serviceFromEncoded(t *testing.T, msg *message.Message) string {
	t.Helper()
	var payload struct {
		Service string `json:"service"`
	}
	if err := json.Unmarshal(msg.GetContent(), &payload); err != nil {
		t.Fatalf("failed to decode encoded message payload: %v\ncontent: %s", err, msg.GetContent())
	}
	return payload.Service
}

// TestAntithesisStaleOriginMetadata_JSONService demonstrates that the JSON
// encoder reads LogSource.Config.Service at encode time (json.go:67), not at
// message creation time. When Config.Service is mutated between message creation
// and encode, the delivered payload carries the wrong (mutated) service.
//
// EXPECTED TO FAIL: asserts service == "svc-a"; actual will be "svc-b".
func TestAntithesisStaleOriginMetadata_JSONService(t *testing.T) {
	// Step 1 — build source with initial service name (container A).
	cfg := &agentconfig.LogsConfig{
		Service: "svc-a",
		Source:  "test-src",
		Type:    "file",
		Path:    "/tmp/test.log",
	}
	src := sources.NewLogSource("container-a", cfg)

	// Step 2 — create message whose Origin points at that source.
	msg := message.NewMessageWithSource([]byte("log line from container A"), "info", src, 0)

	// Step 3 — feed message into processor BEFORE starting it, so we can
	// mutate the source while the message is already buffered in inputChan.
	p, inputChan, outputChan := makeStaleOriginProcessor(JSONEncoder)
	inputChan <- msg

	// Step 4 — simulate container churn: the AD scheduler mutates the
	// Config.Service field on the same LogSource object (or reuses the
	// Config pointer for a new container). The message is already in-flight.
	//
	// In production this race is probabilistic (bounded by AD scan interval,
	// ~5 s). Here we make it deterministic by mutating before the processor
	// goroutine runs.
	src.Config.Service = "svc-b" // container churn / source mutation

	// Step 5 — start processor and let it drain.
	p.Start()
	defer p.Stop()

	// Step 6 — wait for the delivered message.
	select {
	case delivered := <-outputChan:
		gotService := serviceFromEncoded(t, delivered)

		// The service on the delivered wire payload must match the value that
		// was valid when the log line was READ (svc-a), not the value that was
		// set by the churn (svc-b).
		if gotService == "svc-a" {
			t.Logf("NOT A BUG: service correctly captured at creation time: %q", gotService)
		} else {
			t.Fatalf(
				"BUG DEMONSTRATED (log-metadata-not-corrupted / JSON service field): "+
					"message was created with LogSource.Config.Service = %q. "+
					"Config.Service was mutated to %q (simulating container churn) "+
					"while the message was in-flight in the processor's inputChan. "+
					"The JSON encoder (json.go:67) called msg.Origin.Service() at "+
					"encode time and read the MUTATED value. "+
					"Delivered payload has service = %q (expected %q). "+
					"Root cause: Origin.Service() (origin.go:157-159) reads "+
					"o.LogSource.Config.Service directly — no snapshot is taken at "+
					"message creation time. Under container churn the live Config "+
					"field races with in-flight messages.",
				"svc-a", "svc-b", gotService, "svc-a",
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for processor output — message was dropped")
	}
}

// TestAntithesisStaleOriginMetadata_MRFRouting demonstrates that
// filterMRFMessages (processor.go:236) reads LogSource.Config.Service at
// processing time. When Config.Service is mutated between message creation and
// filterMRFMessages execution, the MRF routing decision is based on the wrong
// (mutated) service name.
//
// Scenario:
//   - "billing-service" is in the MRF allowlist → messages should be MRF-tagged.
//   - The source is mutated to "payment-service" while the message is in-flight.
//   - filterMRFMessages reads "payment-service", finds it NOT in the allowlist,
//     and does NOT tag the message IsMRFAllow = true.
//
// EXPECTED TO FAIL: asserts IsMRFAllow == true; actual will be false.
func TestAntithesisStaleOriginMetadata_MRFRouting(t *testing.T) {
	// Step 1 — build source with service that IS in the MRF allowlist.
	cfg := &agentconfig.LogsConfig{
		Service: "billing-service",
		Source:  "billing-src",
		Type:    "file",
		Path:    "/tmp/billing.log",
	}
	src := sources.NewLogSource("billing-container", cfg)

	// Step 2 — create the message.
	msg := message.NewMessageWithSource([]byte("billing log line"), "info", src, 0)

	// Step 3 — build processor with MRF failover active and allowlist = {"billing-service"}.
	inputChan := make(chan *message.Message, 10)
	outputChan := make(chan *message.Message, 10)

	p := &Processor{
		inputChan:       inputChan,
		outputChan:      outputChan,
		processingRules: nil,
		encoder:         JSONEncoder,
		done:            make(chan struct{}, 1),
		diagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		hostname:        nil,
		pipelineMonitor: metrics.NewNoopPipelineMonitor("antithesis-mrf"),
		utilization:     metrics.NewNoopPipelineMonitor("antithesis-mrf").MakeUtilizationMonitor("antithesis-mrf", "antithesis-mrf"),
		instanceID:      "antithesis-mrf",
		configChan:      make(chan failoverConfig, 1),
	}
	// Pre-seed configChan with MRF active and billing-service in the allowlist.
	p.configChan <- failoverConfig{
		isFailoverActive: true,
		failoverServiceAllowlist: map[string]struct{}{
			"billing-service": {},
		},
	}

	// Step 4 — enqueue message.
	inputChan <- msg

	// Step 5 — simulate container churn: billing-service container is replaced
	// by payment-service container before the processor drains the queue.
	src.Config.Service = "payment-service" // churn: same LogSource, new service

	// Step 6 — start processor and let it drain.
	p.Start()
	defer p.Stop()

	// Step 7 — check MRF tag on the delivered message.
	select {
	case delivered := <-outputChan:
		if delivered.IsMRFAllow {
			t.Logf("NOT A BUG: message correctly MRF-tagged based on original service")
		} else {
			t.Fatalf(
				"BUG DEMONSTRATED (log-metadata-not-corrupted / MRF routing): "+
					"message was created with LogSource.Config.Service = %q "+
					"(in MRF allowlist). Config.Service was mutated to %q (simulating "+
					"container churn) while the message was in-flight. "+
					"filterMRFMessages (processor.go:236) called msg.Origin.Service() "+
					"at processing time and read the MUTATED value %q. "+
					"Since %q is NOT in the allowlist, IsMRFAllow was not set. "+
					"The message will be dropped by the MRF secondary region instead "+
					"of being forwarded. "+
					"Root cause: Origin.Service() (origin.go:157-159) reads the live "+
					"LogSource.Config.Service — no snapshot is taken at message creation.",
				"billing-service", "payment-service", "payment-service", "payment-service",
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for processor output — message was dropped")
	}
}
