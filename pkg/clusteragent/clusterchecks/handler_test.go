// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

func (h *Handler) assertLeadershipMessage(t *testing.T, expected state) {
	t.Helper()
	select {
	case value := <-h.leadershipChan:
		assert.Equal(t, expected, value)
	case <-time.After(50 * time.Millisecond):
		assert.Fail(t, "Timeout while waiting for channel message")
	}
}

func (h *Handler) assertNoLeadershipMessage(t *testing.T) {
	t.Helper()
	select {
	case <-h.leadershipChan:
		assert.Fail(t, "Unexpected channel message")
	case <-time.After(50 * time.Millisecond):
		return
	}
}

func TestUpdateLeaderIP(t *testing.T) {
	le := &fakeLeaderEngine{}
	h := &Handler{
		leadershipChan:       make(chan state, 1),
		leaderStatusCallback: le.get,
	}

	// First run, become leader
	le.set("", nil)
	err := h.updateLeaderIP()
	assert.NoError(t, err)
	assert.Equal(t, "", h.leaderIP)
	h.assertLeadershipMessage(t, leader)

	// Second run, still leader, no update
	err = h.updateLeaderIP()
	assert.NoError(t, err)
	assert.Equal(t, "", h.leaderIP)
	h.assertNoLeadershipMessage(t)

	// Query error
	queryError := errors.New("test query error")
	le.set("1.2.3.4", queryError)
	err = h.updateLeaderIP()
	assert.Equal(t, queryError, err)
	assert.Equal(t, "", h.leaderIP)
	h.assertNoLeadershipMessage(t)

	// Lose leadership
	le.set("1.2.3.4", nil)
	err = h.updateLeaderIP()
	assert.NoError(t, err)
	assert.Equal(t, "1.2.3.4", h.leaderIP)
	h.assertLeadershipMessage(t, follower)

	// New leader, still following
	le.set("1.2.3.40", nil)
	err = h.updateLeaderIP()
	assert.NoError(t, err)
	assert.Equal(t, "1.2.3.40", h.leaderIP)
	h.assertNoLeadershipMessage(t)

	// Back to leader
	le.set("", nil)
	err = h.updateLeaderIP()
	assert.NoError(t, err)
	assert.Equal(t, "", h.leaderIP)
	h.assertLeadershipMessage(t, leader)

	// Start fresh, test unknown -> follower
	le = &fakeLeaderEngine{}
	h = &Handler{
		leadershipChan:       make(chan state, 1),
		leaderStatusCallback: le.get,
	}
	le.set("1.2.3.4", nil)
	err = h.updateLeaderIP()
	assert.NoError(t, err)
	assert.Equal(t, "1.2.3.4", h.leaderIP)
	h.assertLeadershipMessage(t, follower)

	// Start fresh, test unknown -> unknown -> leader
	le = &fakeLeaderEngine{}
	h = &Handler{
		leadershipChan:       make(chan state, 1),
		leaderStatusCallback: le.get,
	}
	le.set("", errors.New("failing"))
	for i := 0; i < 4; i++ {
		err = h.updateLeaderIP()
		assert.Error(t, err)
		assert.Equal(t, "", h.leaderIP)
		assert.Equal(t, unknown, h.state)
		h.assertNoLeadershipMessage(t)
	}
	le.set("", nil)
	err = h.updateLeaderIP()
	assert.NoError(t, err)
	assert.Equal(t, "", h.leaderIP)
	assert.Equal(t, leader, h.state)
	h.assertLeadershipMessage(t, leader)
}

// TestHandlerRun tests the full lifecycle of the handling/dispatching
// lifecycle: unknown -> follower -> leader -> follower -> leader -> stop
func TestHandlerRun(t *testing.T) {
	dummyT := &testing.T{}
	ac := &mockedPluggableAutoConfig{}
	ac.Test(t)
	le := &fakeLeaderEngine{
		err: errors.New("failing"),
	}

	h := &Handler{
		autoconfig:           ac,
		leaderStatusFreq:     100 * time.Millisecond,
		warmupDuration:       250 * time.Millisecond,
		leadershipChan:       make(chan state, 1),
		dispatcher:           newDispatcher(),
		leaderStatusCallback: le.get,
		port:                 5005,
	}

	//
	// Initialisation and unknown state
	//

	ctx, cancelRun := context.WithCancel(context.Background())
	runReturned := make(chan struct{}, 1)
	go func() {
		h.Run(ctx)
		runReturned <- struct{}{}
	}()
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		// State is unknown
		h.m.RLock()
		defer h.m.RUnlock()
		return h.state == unknown
	})
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		// API replys not ready
		code, reason := h.ShouldHandle()
		return code == http.StatusServiceUnavailable && reason == notReadyReason
	})

	//
	// Unknown -> follower
	//

	le.set("1.2.3.4", nil)
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		// Internal state change
		h.m.RLock()
		defer h.m.RUnlock()
		return h.state == follower && h.leaderIP == "1.2.3.4"
	})
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		// API redirects to leader
		code, reason := h.ShouldHandle()
		return code == http.StatusFound && reason == "1.2.3.4:5005"
	})

	//
	// Follower -> leader
	//

	le.set("", nil)
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		// Internal state change
		h.m.RLock()
		defer h.m.RUnlock()
		return h.state == leader && h.leaderIP == ""
	})
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		// API serves requests
		code, reason := h.ShouldHandle()
		return code == http.StatusOK && reason == ""
	})
	ac.On("AddScheduler", schedulerName, mock.AnythingOfType("*clusterchecks.dispatcher"), true).Return()
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		// Keep node-agent caches even when timestamp is off (warmup)
		response, err := h.PostStatus("dummy", "10.0.0.1", types.NodeStatus{LastChange: -50})
		return err == nil && response.IsUpToDate == true
	})
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 2*time.Second, func() bool {
		// Test whether we're connected to the AD
		return ac.AssertNumberOfCalls(dummyT, "AddScheduler", 1)
	})

	// Schedule a check and make sure it is assigned to the node "dummy"
	testConfig := integration.Config{
		Name:         "unit_test",
		ClusterCheck: true,
	}
	h.dispatcher.Schedule([]integration.Config{testConfig})
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		// Found one configuration for node dummy
		configs, err := h.GetConfigs("dummy")
		return err == nil && len(configs.Configs) == 1
	})
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		// Flush node-agent caches when timestamp is off
		response, err := h.PostStatus("dummy", "10.0.0.1", types.NodeStatus{LastChange: -50})
		return err == nil && response.IsUpToDate == false
	})

	//
	// Leader -> follower
	//

	ac.On("RemoveScheduler", schedulerName).Return()
	le.set("1.2.3.6", nil)
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		// Internal state change
		h.m.RLock()
		defer h.m.RUnlock()
		return h.state == follower && h.leaderIP == "1.2.3.6"
	})
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		// API redirects to leader again
		code, reason := h.ShouldHandle()
		return code == http.StatusFound && reason == "1.2.3.6:5005"
	})
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 2*time.Second, func() bool {
		// RemoveScheduler is called
		return ac.AssertNumberOfCalls(dummyT, "RemoveScheduler", 1)
	})

	//
	// Follower -> leader again
	//

	le.set("", nil)
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		// API serves requests
		code, reason := h.ShouldHandle()
		return code == http.StatusOK && reason == ""
	})
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		// Dispatcher has been flushed, no config remain
		state, err := h.GetState()
		return err == nil && len(state.Nodes) == 0 && len(state.Dangling) == 0
	})

	h.PostStatus("dummy", "10.0.0.1", types.NodeStatus{})
	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 2*time.Second, func() bool {
		// Test whether we're connected to the AD
		return ac.AssertNumberOfCalls(dummyT, "AddScheduler", 2)
	})

	//
	// Leader -> stop
	//

	ac.On("RemoveScheduler", schedulerName).Return()
	cancelRun()
	select {
	case <-runReturned:
		// All good
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "Timeout while waiting for Run method to end")
	}

	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 2*time.Second, func() bool {
		// RemoveScheduler is called
		return ac.AssertNumberOfCalls(dummyT, "RemoveScheduler", 2)
	})
}
