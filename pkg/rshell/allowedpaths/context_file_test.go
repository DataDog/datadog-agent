// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

// mockRWC is a simple ReadWriteCloser that records how many times Close was called.
type mockRWC struct {
	closeCount atomic.Int32
	closeErr   error
}

func (m *mockRWC) Read(p []byte) (int, error)  { return 0, io.EOF }
func (m *mockRWC) Write(p []byte) (int, error) { return len(p), nil }
func (m *mockRWC) Close() error {
	m.closeCount.Add(1)
	return m.closeErr
}

func TestWithContextClose_ExplicitClose(t *testing.T) {
	ctx := context.Background()
	m := &mockRWC{}
	f := WithContextClose(ctx, m)

	if err := f.Close(); err != nil {
		t.Fatalf("Close() returned unexpected error: %v", err)
	}
	if got := m.closeCount.Load(); got != 1 {
		t.Fatalf("expected Close called once, got %d", got)
	}
}

func TestWithContextClose_ExplicitCloseIdempotent(t *testing.T) {
	ctx := context.Background()
	m := &mockRWC{}
	f := WithContextClose(ctx, m)

	f.Close() //nolint:errcheck
	f.Close() //nolint:errcheck
	f.Close() //nolint:errcheck

	if got := m.closeCount.Load(); got != 1 {
		t.Fatalf("expected Close called exactly once, got %d", got)
	}
}

func TestWithContextClose_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &mockRWC{}
	WithContextClose(ctx, m) //nolint:errcheck

	cancel()

	// The goroutine closes the file asynchronously; poll briefly.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if m.closeCount.Load() == 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("expected Close to be called after context cancellation, got %d calls", m.closeCount.Load())
}

func TestWithContextClose_ContextCancelledBeforeOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done

	m := &mockRWC{}
	WithContextClose(ctx, m) //nolint:errcheck

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if m.closeCount.Load() == 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("expected Close to be called when ctx is already done, got %d calls", m.closeCount.Load())
}

func TestWithContextClose_ContextCancelThenExplicitClose_ClosedOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &mockRWC{}
	f := WithContextClose(ctx, m)

	cancel()
	// Wait for goroutine to fire.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if m.closeCount.Load() >= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// Explicit close after context-triggered close must be a no-op.
	f.Close() //nolint:errcheck

	if got := m.closeCount.Load(); got != 1 {
		t.Fatalf("expected Close called exactly once, got %d", got)
	}
}

func TestWithContextClose_ErrorPropagated(t *testing.T) {
	ctx := context.Background()
	want := errors.New("close failed")
	m := &mockRWC{closeErr: want}
	f := WithContextClose(ctx, m)

	if got := f.Close(); got != want {
		t.Fatalf("expected error %v, got %v", want, got)
	}
}

func TestWithContextClose_GoroutineExitsOnExplicitClose(t *testing.T) {
	// Verify that the background goroutine exits when Close is called even if
	// the context is never cancelled. We test this indirectly by ensuring no
	// extra Close calls happen long after explicit Close.
	ctx := context.Background()
	m := &mockRWC{}
	f := WithContextClose(ctx, m)
	f.Close() //nolint:errcheck

	// Give any stray goroutine time to fire — it should not.
	time.Sleep(10 * time.Millisecond)

	if got := m.closeCount.Load(); got != 1 {
		t.Fatalf("expected exactly one Close, got %d", got)
	}
}
