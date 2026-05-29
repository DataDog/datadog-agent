// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug *demonstration* (not a fix), gated behind the `antithesis_demo`
// build tag so it never runs in normal CI.  Run with:
//
//	go test -tags "antithesis_demo test" \
//	    -run TestAntithesisMultilineRotationDiscard \
//	    ./pkg/logs/tailers/file/ -v
//
// Property under test: multiline-not-split-across-pipelines
//
// Hypothesis (from scratchbook): StopAfterFileRotation() cancels forwardContext
// (via stopForward()) BEFORE decoder.Stop() flushes the aggregated multiline
// event. forwardMessages() selects on <-forwardContext.Done() and discards any
// messages pushed by Flush() after the context is already cancelled. A multiline
// event buffered in the aggregator at rotation time is therefore silently dropped.
//
// Setup — how multiline aggregation is confirmed:
//   - A ProcessingRule of type "multi_line" with pattern "^BEGIN:" is set on the
//     source.  The RegexAggregator in the decoder will hold any line whose content
//     starts with "BEGIN:" as the start of a group, appending all following
//     non-matching (continuation) lines into a single combined message.
//   - A group is only emitted when the NEXT header line arrives or Flush() fires.
//   - We write ONE header + TWO continuation lines and wait for nothing more to
//     arrive before triggering rotation, so the combined event stays buffered.
//   - Control test (TestAntithesisMultilineAggregationControl): same pattern but
//     we wait > flushTimeout and assert the combined event IS delivered.
//
// Rotation test:
//   - closeTimeout is set to 200 ms (well under the 1 s flushTimeout).
//   - Output channel is large (1000) so there is no backpressure — the *only*
//     reason for loss is the forwardContext cancellation race.
//   - We call StopAfterFileRotation() 50 ms after writing, ensuring the timer
//     fires (cancelling forwardContext) before decoder.Stop()/Flush() runs.
//   - We then wait for tailer.done and drain the output channel.
//   - A combined event in the output proves delivery. Its absence proves the bug.
//
// EXPECTED OUTCOME FOR BUG TEST: FAILS — the combined event is discarded.
// EXPECTED OUTCOME FOR CONTROL:  PASSES — the combined event is delivered.

package file

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditormock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

// multilineRotationTestBed sets up a tailer with a multi_line ProcessingRule
// so that lines starting with "BEGIN:" are aggregated with their continuations.
// The flushTimeout is configurable via the global config mock.
// closeTimeout is set by the caller on the returned tailer.
func multilineRotationTestBed(t *testing.T, dir string) (*Tailer, chan *message.Message, *os.File) {
	t.Helper()

	// Use a short aggregation timeout (2 s) so the control test doesn't wait too long,
	// but still >> closeTimeout (200 ms) so the rotation test fires before flush.
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("logs_config.aggregation_timeout", 2000) // 2 000 ms

	re := regexp.MustCompile(`^BEGIN:`)
	rule := &config.ProcessingRule{
		Type:    config.MultiLine,
		Name:    "stacktrace_header",
		Pattern: "BEGIN:",
		Regex:   re,
	}

	path := filepath.Join(dir, fmt.Sprintf("ml-rotation-%d.log", time.Now().UnixNano()))
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create log file: %v", err)
	}

	src := sources.NewReplaceableSource(sources.NewLogSource("", &config.LogsConfig{
		Type:            config.FileType,
		Path:            path,
		ProcessingRules: []*config.ProcessingRule{rule},
	}))
	info := status.NewInfoRegistry()

	outputChan := make(chan *message.Message, 1000) // large — no backpressure

	tailer := NewTailer(&TailerOptions{
		OutputChan:      outputChan,
		File:            NewFile(path, src.UnderlyingSource(), false),
		SleepDuration:   5 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(src, info),
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditormock.NewMockRegistry(),
		FileOpener:      opener.NewFileOpener(),
	})

	return tailer, outputChan, f
}

// drainMessages reads everything currently available on outputChan without blocking.
func drainMessages(outputChan chan *message.Message) []*message.Message {
	var msgs []*message.Message
	for {
		select {
		case m := <-outputChan:
			msgs = append(msgs, m)
		default:
			return msgs
		}
	}
}

// --------------------------------------------------------------------------
// Control test — proves multiline aggregation actually works.
// Writes header + 2 continuation lines, waits > flushTimeout, expects ONE
// combined message with all three lines joined.  If this passes, aggregation
// is engaged and the rotation test result is meaningful.
// --------------------------------------------------------------------------

func TestAntithesisMultilineAggregationControl(t *testing.T) {
	dir := t.TempDir()
	tailer, outputChan, f := multilineRotationTestBed(t, dir)
	defer f.Close()

	tailer.closeTimeout = 60 * time.Second // normal stop, not rotation
	if err := tailer.StartFromBeginning(); err != nil {
		t.Fatalf("start tailer: %v", err)
	}
	defer tailer.Stop()

	// Write: one header + two continuations, then a SECOND header to trigger flush
	// of the first group (ensures aggregation is tested without relying solely on timeout).
	lines := []string{
		"BEGIN: exception in goroutine main\n",
		"    at pkg/foo/bar.go:42\n",
		"    at pkg/foo/baz.go:17\n",
		"BEGIN: second event (triggers flush of first)\n",
	}
	for _, l := range lines {
		if _, err := f.WriteString(l); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// Wait for the first combined event to arrive (flush triggered by second header).
	var got *message.Message
	select {
	case got = <-outputChan:
	case <-time.After(3 * time.Second):
		t.Fatal("CONTROL FAILED: no message received within 3s — multiline aggregation did not engage")
	}

	content := string(got.GetContent())
	t.Logf("CONTROL: received message: %q", content)

	if !strings.HasPrefix(content, "BEGIN: exception") {
		t.Fatalf("CONTROL FAILED: first message does not start with header: %q", content)
	}
	if !strings.Contains(content, "pkg/foo/bar.go") || !strings.Contains(content, "pkg/foo/baz.go") {
		t.Fatalf("CONTROL FAILED: continuation lines missing from aggregated message: %q", content)
	}

	t.Logf("CONTROL PASSED: aggregation confirmed — header + continuations combined into one message")
}

// --------------------------------------------------------------------------
// Bug demonstration — rotation discards buffered multiline event.
// Writes header + 2 continuation lines (NO following header), triggers
// StopAfterFileRotation() 50 ms later (before flushTimeout fires), and
// checks whether the combined event is delivered.
// EXPECTED TO FAIL — demonstrating the silent discard.
// --------------------------------------------------------------------------

func TestAntithesisMultilineRotationDiscard(t *testing.T) {
	dir := t.TempDir()
	tailer, outputChan, f := multilineRotationTestBed(t, dir)
	defer f.Close()

	// closeTimeout << flushTimeout (2 s): the stopForward goroutine cancels
	// forwardContext ~200 ms after rotation, well before decoder.Stop()/Flush()
	// would emit the buffered group.
	tailer.closeTimeout = 200 * time.Millisecond

	if err := tailer.StartFromBeginning(); err != nil {
		t.Fatalf("start tailer: %v", err)
	}

	// Write the multiline event: header + 2 continuation lines.
	// No second header follows — the group stays buffered in RegexAggregator.
	event := []string{
		"BEGIN: NullPointerException in thread main\n",
		"    at com.example.Foo.run(Foo.java:10)\n",
		"    at com.example.Bar.call(Bar.java:5)\n",
	}
	for _, l := range event {
		if _, err := f.WriteString(l); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// Let the tailer read all three lines (sleepDuration is 5 ms; 50 ms is plenty).
	time.Sleep(50 * time.Millisecond)

	// Simulate file rotation.  closeTimeout=200 ms means stopForward() will fire
	// ~200 ms from now, cancelling forwardContext BEFORE decoder.Stop()/Flush().
	tailer.StopAfterFileRotation()

	// Wait for the tailer to finish.
	select {
	case <-tailer.done:
	case <-time.After(tailer.closeTimeout + 5*time.Second):
		t.Fatal("tailer did not stop within the expected window")
	}

	// Drain whatever made it to the output channel.
	msgs := drainMessages(outputChan)
	t.Logf("received %d message(s) after rotation", len(msgs))
	for i, m := range msgs {
		t.Logf("  msg[%d]: %q", i, string(m.GetContent()))
	}

	// Look for the combined multiline event.
	var found bool
	for _, m := range msgs {
		c := string(m.GetContent())
		if strings.HasPrefix(c, "BEGIN: NullPointerException") &&
			strings.Contains(c, "Foo.java:10") &&
			strings.Contains(c, "Bar.java:5") {
			found = true
			t.Logf("combined event delivered: %q", c)
			break
		}
	}

	if !found {
		// Report what (if anything) was delivered.
		var delivered []string
		for _, m := range msgs {
			delivered = append(delivered, string(m.GetContent()))
		}
		t.Fatalf(
			"BUG DEMONSTRATED (multiline-not-split-across-pipelines): "+
				"multiline event buffered in RegexAggregator at rotation time was "+
				"silently discarded — stopForward() cancelled forwardContext before "+
				"decoder.Stop()/Flush() could push the combined event. "+
				"Messages actually delivered (%d): %v",
			len(delivered), delivered,
		)
	}

	t.Logf("HYPOTHESIS REFUTED: combined multiline event was delivered despite rotation")
}
