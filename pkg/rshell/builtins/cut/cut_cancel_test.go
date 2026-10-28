// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cut

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// countingWriter counts Write calls so tests can tell how far a per-line
// loop got before it noticed cancellation.
type countingWriter struct{ writes int }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.writes++
	return len(p), nil
}

// A single line can hold about a million fields or ranges, so the per-line
// loops must stop on a cancelled context instead of finishing the line.
func TestPerLineLoopsStopWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fieldsLine := []byte(strings.Repeat(",", 10_000))
	bytesLine := []byte(strings.Repeat("a", 10_000))
	var odd [][2]int
	for i := 1; i <= len(bytesLine); i += 2 {
		odd = append(odd, [2]int{i, i})
	}

	cases := []struct {
		name string
		run  func(callCtx *builtins.CallContext) error
	}{
		{"fields", func(c *builtins.CallContext) error {
			cfg := &cutConfig{mode: modeFields, ranges: [][2]int{{1, 1 << 31}}, delimByte: ',', outDelim: []byte("::"), outDelimSet: true}
			return processFields(ctx, c, fieldsLine, cfg)
		}},
		{"bytes with output delimiter", func(c *builtins.CallContext) error {
			cfg := &cutConfig{mode: modeBytes, ranges: odd, outDelim: []byte("::"), outDelimSet: true}
			return processBytes(ctx, c, bytesLine, cfg)
		}},
		{"bytes complement with output delimiter", func(c *builtins.CallContext) error {
			cfg := &cutConfig{mode: modeBytes, ranges: odd, complement: true, outDelim: []byte("::"), outDelimSet: true}
			return processBytes(ctx, c, bytesLine, cfg)
		}},
		{"bytes", func(c *builtins.CallContext) error {
			cfg := &cutConfig{mode: modeBytes, ranges: odd}
			return processBytes(ctx, c, bytesLine, cfg)
		}},
		{"bytes complement", func(c *builtins.CallContext) error {
			cfg := &cutConfig{mode: modeBytes, ranges: odd, complement: true}
			return processBytes(ctx, c, bytesLine, cfg)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := &countingWriter{}
			err := tc.run(&builtins.CallContext{Stdout: w, Stderr: &bytes.Buffer{}})
			assert.ErrorIs(t, err, context.Canceled)
			assert.Zero(t, w.writes)
		})
	}
}
