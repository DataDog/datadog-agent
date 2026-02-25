// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/mocktracer"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

// filterSpans returns all finished spans with the given operation name.
func filterSpans(spans []mocktracer.Span, opName string) []mocktracer.Span {
	var result []mocktracer.Span
	for _, s := range spans {
		if s.OperationName() == opName {
			result = append(result, s)
		}
	}
	return result
}

// TestHandlerWarmupSpan verifies that a leader_warmup span is emitted when
// the handler completes warmup normally, and that a leadership_lost span is
// emitted when leadership is subsequently lost.
func TestHandlerWarmupSpan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Cluster Agent is not supported on Windows")
	}

	mt := mocktracer.Start()
	defer mt.Stop()

	ac := &mockedPluggableAutoConfig{}
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	ac.Test(t)
	le := &fakeLeaderEngine{err: errors.New("failing")}

	h := &Handler{
		autoconfig:           ac,
		leaderStatusFreq:     50 * time.Millisecond,
		warmupDuration:       100 * time.Millisecond,
		leadershipChan:       make(chan state, 1),
		dispatcher:           newDispatcher(fakeTagger),
		leaderStatusCallback: le.get,
	}

	ctx, cancelRun := context.WithCancel(context.Background())
	runReturned := make(chan struct{}, 1)
	go func() {
		h.Run(ctx)
		runReturned <- struct{}{}
	}()

	// Become leader and let warmup complete normally.
	ac.On("AddScheduler", schedulerName, mock.AnythingOfType("*clusterchecks.dispatcher"), true).Return()
	le.set("", nil)
	testutil.AssertTrueBeforeTimeout(t, tick, waitfor, func() bool {
		return ac.AssertNumberOfCalls(&testing.T{}, "AddScheduler", 1)
	})

	warmupSpans := filterSpans(mt.FinishedSpans(), "cluster_checks.handler.leader_warmup")
	require.Len(t, warmupSpans, 1, "expected one warmup span after normal warmup completion")
	assert.Nil(t, warmupSpans[0].Tag("interrupted"), "uninterrupted warmup should have no interrupted tag")

	// Lose leadership — triggers leadership_lost span.
	ac.On("RemoveScheduler", schedulerName).Return()
	le.set("127.0.0.1", nil)
	testutil.AssertTrueBeforeTimeout(t, tick, waitfor, func() bool {
		return ac.AssertNumberOfCalls(&testing.T{}, "RemoveScheduler", 1)
	})

	lostSpans := filterSpans(mt.FinishedSpans(), "cluster_checks.handler.leadership_lost")
	require.Len(t, lostSpans, 1, "expected one leadership_lost span after losing leadership")

	cancelRun()
	select {
	case <-runReturned:
	case <-time.After(2 * time.Second):
		assert.Fail(t, "timeout waiting for Run to return")
	}
}
