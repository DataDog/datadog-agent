// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startupsequencerimpl

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	startupsequencer "github.com/DataDog/datadog-agent/comp/core/startupsequencer/def"
	"github.com/DataDog/datadog-agent/pkg/util/stagedstart"
)

// newTestSequencer builds a sequencer whose pacer is a no-op (no delay, no
// reclaim) so ordering/inline tests run instantly.
func newTestSequencer(t *testing.T, enabled bool) *sequencer {
	cfg := stagedstart.Config{Enabled: enabled}
	return &sequencer{
		log:     logmock.New(t),
		enabled: enabled,
		pacer:   stagedstart.NewPacer(cfg, nil, nil),
	}
}

// When disabled, Defer must run the work synchronously and propagate its error.
func TestDisabledRunsInline(t *testing.T) {
	s := newTestSequencer(t, false)

	ran := false
	err := s.Defer(startupsequencer.StageBackground, "x", func(context.Context) error {
		ran = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, ran, "work should run synchronously when staging is disabled")

	sentinel := errors.New("boom")
	err = s.Defer(startupsequencer.StageChecks, "y", func(context.Context) error { return sentinel })
	assert.ErrorIs(t, err, sentinel, "error must propagate when staging is disabled")

	s.Begin(context.Background())
}

// When enabled, Defer must not run the work until Begin, and the work must run
// in ascending stage order.
func TestEnabledRunsInStageOrder(t *testing.T) {
	s := newTestSequencer(t, true)

	var mu sync.Mutex
	var order []string
	record := func(name string) func(context.Context) error {
		return func(context.Context) error {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return nil
		}
	}

	done := make(chan struct{})
	require.NoError(t, s.Defer(startupsequencer.StageBackground, "background", func(ctx context.Context) error {
		_ = record("background")(ctx)
		close(done)
		return nil
	}))
	require.NoError(t, s.Defer(startupsequencer.StageCritical, "critical", record("critical")))
	require.NoError(t, s.Defer(startupsequencer.StageIngest, "ingest", record("ingest")))

	mu.Lock()
	assert.Empty(t, order, "no deferred work should run before Begin")
	mu.Unlock()

	s.Begin(context.Background())

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("staged startup did not complete in time")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"critical", "ingest", "background"}, order)
}

// Work registered after the sequence has begun must still run (inline).
func TestLateRegistrationRunsInline(t *testing.T) {
	s := newTestSequencer(t, true)
	s.Begin(context.Background())

	ran := false
	require.NoError(t, s.Defer(startupsequencer.StageChecks, "late", func(context.Context) error {
		ran = true
		return nil
	}))
	assert.True(t, ran, "work registered after Begin should run inline")
}

// A cancelled context must stop the sequence rather than running later items.
func TestContextCancellationStops(t *testing.T) {
	s := newTestSequencer(t, true)
	// A long pacer interval makes the sequencer block between items so we can
	// cancel mid-sequence.
	s.pacer = stagedstart.NewPacer(stagedstart.Config{Enabled: true, Interval: time.Hour}, nil, nil)

	first := make(chan struct{})
	require.NoError(t, s.Defer(startupsequencer.StageCritical, "first", func(context.Context) error {
		close(first)
		return nil
	}))
	secondRan := false
	require.NoError(t, s.Defer(startupsequencer.StageIngest, "second", func(context.Context) error {
		secondRan = true
		return nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	s.Begin(ctx)

	<-first  // first item ran; sequencer now paused before the second
	cancel() // cancel during the inter-item wait

	time.Sleep(50 * time.Millisecond)
	assert.False(t, secondRan, "later item must not run after cancellation")
}
