// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug investigation, gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" -run TestAntithesisAuditorDrainsOnStop \
//	    ./comp/logs/auditor/impl/ -v -count=1 2>&1 | grep -v "^[0-9]\{16\} \[Info\]"
//
// Property under test: `auditor-drains-on-stop`
//
// # Claimed bug (from scratchbook test/antithesis/scratchbook/properties/auditor-drains-on-stop.md)
//
// The run loop uses `select` over inputChan (auditor.go:283-333). When Stop()
// closes inputChan with N buffered payloads, Go's select chooses uniformly at
// random among ready cases. If flushTicker.C or cleanUpTicker.C is also ready,
// the loop may consume a tick instead of a buffered payload. On the next iteration
// it sees isOpen=false and returns without processing the remaining items. Those
// payloads are never written to the registry → stale offsets → duplicates on restart.
//
// # Verdict: NOT A BUG (drain-on-stop property holds by Go channel semantics)
//
// The scratchbook analysis is incorrect in one key detail. In Go, receiving from
// a closed channel with N buffered items ALWAYS delivers those N items with
// isOpen=true before delivering the zero value with isOpen=false. The `select`
// statement may pick ticker cases between payload deliveries, but it cannot
// cause isOpen=false to fire while items remain buffered. Go's specification
// guarantees ordering within a single channel.
//
// Proof via /tmp/chantest.go (run independently):
//
//	ch := make(chan int, 3); ch <- 1; ch <- 2; ch <- 3; close(ch)
//	other := make(chan int, 1); other <- 999
//	// select iteration 1: picks other (tick arm) — ch still has 1,2,3 buffered
//	// select iteration 2: picks ch → 1 (isOpen=true)
//	// select iteration 3: picks ch → 2 (isOpen=true)
//	// select iteration 4: picks ch → 3 (isOpen=true)
//	// select iteration 5: picks ch → 0 (isOpen=false) ← only now
//
// The critical invariant: "other" stealing one iteration does NOT skip any
// buffered items — they remain in ch until explicitly received. After all N
// items are consumed, the (N+1)-th receive yields isOpen=false.
//
// Therefore: Stop() → closeChannels() → close(inputChan) + <-a.done guarantees
// all items buffered at close-time are drained by the run loop before a.done
// fires, and before flushRegistry() writes the registry.
//
// # Real remaining race: Flush() H2 snapshot race (separate from Stop())
//
// The Flush() arm (auditor.go:313-331) snapshots n := len(a.inputChan) and
// drains exactly n items. Items arriving between the snapshot and drain are
// missed by this Flush() call. Stop() does NOT call Flush(), so this H2 race
// does not affect the drain-on-stop property. It is relevant to transport-
// restart scenarios where Flush() is called before restarting destinations.
//
// # Test sub-cases
//
// Part 1 (StopDrainsAllBufferedPayloads): fills inputChan with 100 payloads,
// calls Stop(), verifies all 100 appear in the registry. EXPECTED TO PASS.
// Failure would indicate drain-on-stop bug (unexpected).
//
// Part 2 (FlushH2RaceSnapshotMissesLateArrivals): informational test exercising
// the Flush() H2 race. Does not use t.Fatalf; logs race observations only.
//
// Key code locations (comp/logs/auditor/impl/auditor.go):
//   - Stop()         line 132: closeChannels() then flushRegistry()
//   - closeChannels() line 171: close(inputChan) + <-done
//   - run() payload  line 285: case payload, isOpen := <-a.inputChan
//   - run() flush    line 313: n := len(a.inputChan) ← H2 snapshot race

package auditorimpl

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	kubehealthmock "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/mock"
	agentconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// buildAuditorForDemo creates a fresh registryAuditor with a 100-slot channel
// (the production default from logs_config.message_channel_size) and a temp dir.
func buildAuditorForDemo(t *testing.T) *registryAuditor {
	t.Helper()
	dir := t.TempDir()
	cfg := configmock.NewMock(t)
	cfg.SetWithoutSource("logs_config.run_path", dir)
	cfg.SetWithoutSource("logs_config.message_channel_size", 100)
	cfg.SetWithoutSource("logs_config.auditor_ttl", 24)

	deps := Dependencies{
		Config:     cfg,
		Log:        logmock.New(t),
		KubeHealth: kubehealthmock.NewMockRegistrar(),
	}
	return newAuditor(deps)
}

// buildPayload wraps a single message into a *message.Payload with the given
// identifier and offset — the two fields that the run loop writes to the registry.
func buildPayload(identifier, offset string, ts int64) *message.Payload {
	src := sources.NewLogSource("demo", &agentconfig.LogsConfig{
		Path:        identifier,
		TailingMode: "end",
	})
	origin := message.NewOrigin(src)
	origin.Identifier = identifier
	origin.Offset = offset

	meta := &message.MessageMetadata{
		Origin:             origin,
		Status:             message.StatusInfo,
		IngestionTimestamp: ts,
	}
	return message.NewPayload([]*message.MessageMetadata{meta}, nil, "", 0)
}

// TestAntithesisAuditorDrainsOnStop is an Antithesis-style demonstration test
// for the `auditor-drains-on-stop` property.
//
// Sub-test 1 (StopDrainsAllBufferedPayloads): fills inputChan with N payloads,
// calls Stop(), asserts all N offsets are present in the registry.
// EXPECTED TO PASS — the property holds by Go closed-channel semantics.
// If it fails, the drain-on-stop bug is present.
//
// Sub-test 2 (FlushH2Race): informational. Sends a payload then immediately
// calls Flush(), observing whether the Flush() snapshot missed the payload.
// Does not assert failure; logs race observations only.
func TestAntithesisAuditorDrainsOnStop(t *testing.T) {
	t.Run("StopDrainsAllBufferedPayloads", func(t *testing.T) {
		// Fill inputChan with N=100 payloads (the full production channel capacity).
		// The run loop may have drained some by the time Stop() is called —
		// we only care that all N appear in the registry after Stop() returns.
		//
		// WHY THIS WORKS (invariant): Stop() calls closeChannels(), which calls
		// close(inputChan) and then blocks on <-a.done. The run loop goroutine
		// only writes a.done AFTER the for-select exits. The for-select exits
		// only when case payload, isOpen := <-a.inputChan fires with isOpen=false.
		// In Go, isOpen=false is only returned AFTER all N buffered items have
		// been received. Therefore all items in inputChan at the time of close()
		// are processed by the run loop before a.done is written, before
		// closeChannels() returns, before flushRegistry() is called.
		//
		// The `select` can pick flushTicker.C or cleanUpTicker.C between items,
		// consuming additional iterations, but this does NOT skip any buffered
		// items — they remain in the channel until the payload case receives them.
		//
		// Expected outcome: PASS.
		// Failure = drain-on-stop bug demonstrated.

		const N = 100 // full channel capacity (logs_config.message_channel_size default)

		a := buildAuditorForDemo(t)
		a.Start()

		ch := a.Channel()
		for i := 0; i < N; i++ {
			id := fmt.Sprintf("file:/var/log/drain-test-%04d.log", i)
			off := strconv.Itoa((i + 1) * 100)
			payload := buildPayload(id, off, int64(i+1))
			select {
			case ch <- payload:
			case <-time.After(5 * time.Second):
				t.Fatalf("timed out sending payload %d/%d", i, N)
			}
		}

		bufferedBeforeStop := len(ch)
		t.Logf("len(inputChan) immediately before Stop(): %d/%d", bufferedBeforeStop, N)

		// Stop() closes inputChan and blocks until the run loop exits via <-done.
		// By the time Stop() returns, all N items must have been processed.
		a.Stop()

		lost := 0
		for i := 0; i < N; i++ {
			id := fmt.Sprintf("file:/var/log/drain-test-%04d.log", i)
			want := strconv.Itoa((i + 1) * 100)
			got := a.GetOffset(id)
			if got != want {
				lost++
				if lost <= 5 {
					t.Logf("  MISSING: %s want=%q got=%q", id, want, got)
				}
			}
		}

		if lost > 0 {
			t.Fatalf(
				"BUG DEMONSTRATED (auditor-drains-on-stop): Stop() failed to drain "+
					"%d/%d payloads. Those offsets are stale in the registry — "+
					"the corresponding log lines will be re-read and re-delivered "+
					"as duplicates on restart. "+
					"bufferedBeforeStop=%d lost=%d survived=%d",
				lost, N, bufferedBeforeStop, lost, N-lost,
			)
		}

		t.Logf("PROPERTY HOLDS: all %d/%d payloads drained to registry by Stop().", N, N)
		t.Logf("Proof: Go closed-channel semantics guarantee all N buffered items "+
			"are delivered (isOpen=true) before the zero-value (isOpen=false). "+
			"The select's random choice between ready cases cannot skip buffered "+
			"items — it can only interleave ticker iterations between them.")
	})

	t.Run("FlushH2Race", func(t *testing.T) {
		// Informational: demonstrate the separate H2 race in Flush().
		// The Flush() arm snapshots n := len(a.inputChan) and drains exactly n
		// items. An item arriving after the snapshot is missed by this Flush().
		// Stop() does NOT call Flush(), so this race does not affect drain-on-stop.
		// It matters in transport-restart scenarios.
		//
		// Strategy: ensure the channel is empty (run loop has drained prior items),
		// then enqueue one payload and immediately call Flush(). If the flush arm's
		// snapshot executes before the channel send completes, n=0 and the payload
		// is not processed in this flush call.

		a := buildAuditorForDemo(t)
		a.Start()
		defer a.Stop()

		const attempts = 20
		h2Triggered := 0

		for i := 0; i < attempts; i++ {
			id := fmt.Sprintf("file:/var/log/h2-race-%04d.log", i)
			want := strconv.Itoa((i + 1) * 100)

			// Send payload then immediately flush — race the snapshot.
			ch := a.Channel()
			payload := buildPayload(id, want, int64(i+1))
			ch <- payload
			a.Flush() // may or may not snapshot the payload

			got := a.GetOffset(id)
			if got != want {
				h2Triggered++
				t.Logf("H2 race attempt %d: payload NOT in registry after Flush() "+
					"(want=%q got=%q) — Flush() snapshot missed this item.",
					i, want, got)
			}
		}

		if h2Triggered > 0 {
			t.Logf("H2 race in Flush() observed %d/%d times: items sent before "+
				"Flush() was called were not processed because len(inputChan) was "+
				"snapshotted at 0 before the payload arrived in the channel. "+
				"This is a separate bug from drain-on-stop; Stop() does not call "+
				"Flush(), so the Stop() drain path is unaffected.",
				h2Triggered, attempts)
		} else {
			t.Logf("H2 race in Flush() not triggered in %d attempts. "+
				"The payload was enqueued before the flush snapshot in all cases.",
				attempts)
		}
		// No t.Fatalf: this sub-test is informational about the H2 race in Flush(),
		// which is outside the drain-on-stop property scope.
	})
}
