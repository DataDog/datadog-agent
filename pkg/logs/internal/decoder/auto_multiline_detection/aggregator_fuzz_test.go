// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// FuzzAggregatorGrouping ensures aggregation matches the label semantics when size limits are not hit.
func FuzzAggregatorGrouping(f *testing.F) {
	f.Add([]byte("abc"), []byte{0, 1, 2})

	f.Fuzz(func(t *testing.T, payload []byte, lbls []byte) {
		if len(payload) == 0 || len(lbls) == 0 {
			return
		}

		// Build deterministic messages from payload bytes.
		var msgs []string
		for i := 0; i < len(payload) && len(msgs) < 8; i++ {
			runLen := int(payload[i]%5) + 1
			char := 'a' + (payload[i] % 26)
			msgs = append(msgs, strings.Repeat(string(rune(char)), runLen))
		}
		if len(msgs) == 0 {
			return
		}

		labelFor := func(i int) Label {
			switch lbls[i%len(lbls)] % 3 {
			case 0:
				return startGroup
			case 1:
				return aggregate
			default:
				return noAggregate
			}
		}

		// Model expected grouping (maxContentSize large enough to avoid truncation).
		var expected [][]string
		var current []string
		flush := func() {
			if len(current) > 0 {
				expected = append(expected, current)
				current = nil
			}
		}

		for i, m := range msgs {
			switch labelFor(i) {
			case noAggregate:
				flush()
				expected = append(expected, []string{m})
			case startGroup:
				flush()
				current = []string{m}
			case aggregate:
				if len(current) == 0 {
					expected = append(expected, []string{m})
				} else {
					current = append(current, m)
				}
			}
		}
		flush()

		outputCh := make(chan *message.Message, len(expected)+2)
		ag := NewAggregator(func(m *message.Message) { outputCh <- m }, 1<<15, false, false, status.NewInfoRegistry())

		for i, m := range msgs {
			ag.Aggregate(newMessage(m), labelFor(i))
		}
		ag.Flush()

		var outputs []string
		for i := 0; i < len(expected); i++ {
			out := <-outputCh
			outputs = append(outputs, string(out.GetContent()))
			wantMulti := len(expected[i]) > 1
			if wantMulti != out.ParsingExtra.IsMultiLine {
				t.Fatalf("multiline flag mismatch: want %v got %v content=%q expectedParts=%v", wantMulti, out.ParsingExtra.IsMultiLine, out.GetContent(), expected[i])
			}
			if wantMulti && !bytes.Contains(out.GetContent(), message.EscapedLineFeed) {
				t.Fatalf("expected escaped newline separator in multiline output %q", out.GetContent())
			}
			if !wantMulti && bytes.Contains(out.GetContent(), message.EscapedLineFeed) {
				t.Fatalf("unexpected escaped newline in single-line output %q", out.GetContent())
			}
		}

		if len(outputs) != len(expected) {
			t.Fatalf("output count mismatch: got %d want %d", len(outputs), len(expected))
		}
		for i, exp := range expected {
			wantContent := strings.Join(exp, string(message.EscapedLineFeed))
			if outputs[i] != wantContent {
				t.Fatalf("content mismatch at %d: got %q want %q (parts=%v)", i, outputs[i], wantContent, exp)
			}
		}
	})
}
