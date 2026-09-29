// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

// filterSpans returns all finished spans with the given operation name.
func filterSpans(spans []*mocktracer.Span, opName string) []*mocktracer.Span {
	var result []*mocktracer.Span
	for _, s := range spans {
		if s.OperationName() == opName {
			result = append(result, s)
		}
	}
	return result
}

// TestHandlerLeadershipEmitsNoSpans verifies that warmup and leadership
// transitions are logged rather than traced.
func TestHandlerLeadershipEmitsNoSpans(t *testing.T) {
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

	// Lose leadership.
	ac.On("RemoveScheduler", schedulerName).Return()
	le.set("127.0.0.1", nil)
	testutil.AssertTrueBeforeTimeout(t, tick, waitfor, func() bool {
		return ac.AssertNumberOfCalls(&testing.T{}, "RemoveScheduler", 1)
	})

	assert.Empty(t, filterSpans(mt.FinishedSpans(), "cluster_checks.handler.leader_warmup"))
	assert.Empty(t, filterSpans(mt.FinishedSpans(), "cluster_checks.handler.leadership_lost"))

	cancelRun()
	select {
	case <-runReturned:
	case <-time.After(2 * time.Second):
		assert.Fail(t, "timeout waiting for Run to return")
	}
}

func TestScheduleSpanCheckNamesBounded(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	d := newDispatcher(fakeTagger)

	var configs []integration.Config
	for i := 0; i < 3*maxCheckNamesTag; i++ {
		configs = append(configs, integration.Config{Name: fmt.Sprintf("check_%02d", i)})
		configs = append(configs, integration.Config{Name: fmt.Sprintf("check_%02d", i)})
	}
	d.Schedule(configs)

	spans := filterSpans(mt.FinishedSpans(), "cluster_checks.dispatcher.schedule")
	require.Len(t, spans, 1)
	assert.Equal(t, float64(len(configs)), spans[0].Tag("config_count"))
	assert.Equal(t, "check_00,check_01,check_02,check_03,check_04,check_05,check_06,check_07,check_08,check_09,...", spans[0].Tag("check_names"))
}
