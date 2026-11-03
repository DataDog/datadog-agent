// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// vuln-hunt campaign: 2026-05-19-codex
// Target: read (builtin)

package read

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type infiniteByteReader struct {
	b byte
}

func (r infiniteByteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.b
	}
	return len(p), nil
}

func TestVulnHuntBuiltinResourceExhaustion_BackslashContinuationConsumedCap(t *testing.T) {
	input := strings.Repeat("\\\n", MaxReadBytes/2+2)
	line, eof, err := readInput(context.Background(), strings.NewReader(input), '\n', false, -1, false, false)
	if err == nil {
		t.Fatalf("expected consumed-input cap error, got line length=%d eof=%v", len(line), eof)
	}
	if got, want := err.Error(), fmt.Sprintf("input exceeds maximum of %d bytes", MaxReadBytes); got != want {
		t.Fatalf("got err=%q, want %q", got, want)
	}
	if line != "" {
		t.Fatalf("continuation-only input should not append data, got %q", line)
	}
}

func TestVulnHuntBuiltinSpecialFiles_NULStreamConsumedCap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		line, eof, err := readInput(ctx, infiniteByteReader{b: 0}, '\n', true, -1, false, false)
		if line != "" {
			done <- fmt.Errorf("NUL-only stream should not append data, got %q", line)
			return
		}
		if eof {
			done <- errors.New("infinite stream unexpectedly returned EOF")
			return
		}
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected consumed-input cap or context error")
		}
		want := fmt.Sprintf("input exceeds maximum of %d bytes", MaxReadBytes)
		if err.Error() != want && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("readInput did not return for an infinite NUL stream")
	}
}
