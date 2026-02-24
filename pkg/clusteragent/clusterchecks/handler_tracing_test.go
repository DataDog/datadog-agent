// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/mocktracer"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
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
// the handler completes warmup normally, and that it carries an "interrupted"
// tag when warmup is cut short by a leadership loss.
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

	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "I'm a teapot", 418)
	}))
	testPort := testServer.Listener.Addr().(*net.TCPAddr).Port
	defer testServer.Close()

	h := &Handler{
		autoconfig:           ac,
		leaderStatusFreq:     50 * time.Millisecond,
		warmupDuration:       100 * time.Millisecond,
		leadershipChan:       make(chan state, 1),
		dispatcher:           newDispatcher(fakeTagger),
		leaderStatusCallback: le.get,
		leaderForwarder:      api.NewLeaderForwarder(testPort, 10),
	}
	h.dispatcher.tracingEnabled = true

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
		h.m.RLock()
		defer h.m.RUnlock()
		return h.state == follower
	})
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

// TestHandlerWarmupSpanInterrupted verifies that the warmup span carries
// interrupted="leadership_lost" when leadership is lost before warmup ends.
func TestHandlerWarmupSpanInterrupted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Cluster Agent is not supported on Windows")
	}

	mt := mocktracer.Start()
	defer mt.Stop()

	ac := &mockedPluggableAutoConfig{}
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	ac.Test(t)
	le := &fakeLeaderEngine{err: errors.New("failing")}

	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "I'm a teapot", 418)
	}))
	testPort := testServer.Listener.Addr().(*net.TCPAddr).Port
	defer testServer.Close()

	h := &Handler{
		autoconfig:           ac,
		leaderStatusFreq:     50 * time.Millisecond,
		warmupDuration:       10 * time.Second, // long warmup — we'll interrupt it
		leadershipChan:       make(chan state, 1),
		dispatcher:           newDispatcher(fakeTagger),
		leaderStatusCallback: le.get,
		leaderForwarder:      api.NewLeaderForwarder(testPort, 10),
	}
	h.dispatcher.tracingEnabled = true

	ctx, cancelRun := context.WithCancel(context.Background())
	runReturned := make(chan struct{}, 1)
	go func() {
		h.Run(ctx)
		runReturned <- struct{}{}
	}()

	// Become leader — warmup starts but won't finish for 10s.
	le.set("", nil)
	testutil.AssertTrueBeforeTimeout(t, tick, waitfor, func() bool {
		h.m.RLock()
		defer h.m.RUnlock()
		return h.state == leader
	})

	// Immediately lose leadership to interrupt the warmup.
	le.set("127.0.0.1", nil)
	testutil.AssertTrueBeforeTimeout(t, tick, waitfor, func() bool {
		h.m.RLock()
		defer h.m.RUnlock()
		return h.state == follower
	})

	warmupSpans := filterSpans(mt.FinishedSpans(), "cluster_checks.handler.leader_warmup")
	require.Len(t, warmupSpans, 1, "expected one warmup span even when interrupted")
	assert.Equal(t, "leadership_lost", warmupSpans[0].Tag("interrupted"))

	cancelRun()
	select {
	case <-runReturned:
	case <-time.After(2 * time.Second):
		assert.Fail(t, "timeout waiting for Run to return")
	}
}
