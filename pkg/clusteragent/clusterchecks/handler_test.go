// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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
)

func (h *Handler) setCallback(ip string, err error) {
	h.m.Lock()
	defer h.m.Unlock()
	h.leaderStatusCallback = func() (string, error) { return ip, err }
}

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
	h := &Handler{
		leadershipChan: make(chan state, 1),
	}

	// First run, become leader
	h.setCallback("", nil)
	err := h.updateLeaderIP(true)
	assert.NoError(t, err)
	assert.Equal(t, "", h.leaderIP)
	h.assertLeadershipMessage(t, leader)

	// Second run, still leader, no update
	err = h.updateLeaderIP(false)
	assert.NoError(t, err)
	assert.Equal(t, "", h.leaderIP)
	h.assertNoLeadershipMessage(t)

	// Query error
	queryError := errors.New("test query error")
	h.setCallback("1.2.3.4", queryError)
	err = h.updateLeaderIP(false)
	assert.Equal(t, queryError, err)
	assert.Equal(t, "", h.leaderIP)
	h.assertNoLeadershipMessage(t)

	// Lose leadership
	h.setCallback("1.2.3.4", nil)
	err = h.updateLeaderIP(false)
	assert.NoError(t, err)
	assert.Equal(t, "1.2.3.4", h.leaderIP)
	h.assertLeadershipMessage(t, follower)

	// New leader, still following
	h.setCallback("1.2.3.40", nil)
	err = h.updateLeaderIP(false)
	assert.NoError(t, err)
	assert.Equal(t, "1.2.3.40", h.leaderIP)
	h.assertNoLeadershipMessage(t)

	// Back to leader
	h.setCallback("", nil)
	err = h.updateLeaderIP(false)
	assert.NoError(t, err)
	assert.Equal(t, "", h.leaderIP)
	h.assertLeadershipMessage(t, leader)

	// Start fresh, test unknown -> follower
	h = &Handler{
		leadershipChan: make(chan state, 1),
	}
	h.setCallback("1.2.3.4", nil)
	err = h.updateLeaderIP(true)
	assert.NoError(t, err)
	assert.Equal(t, "1.2.3.4", h.leaderIP)
	h.assertLeadershipMessage(t, follower)
}

// TestHandlerRun tests the full lifecycle of the handling/dispatching
// lifecycle: unknown -> follower -> leader -> follower -> leader -> stop
func TestHandlerRun(t *testing.T) {
	dummyT := &testing.T{}
	ac := &mockedPluggableAutoConfig{}
	ac.Test(t)

	h := &Handler{
		autoconfig:       ac,
		leaderStatusFreq: 100 * time.Millisecond,
		warmupDuration:   250 * time.Millisecond,
		leadershipChan:   make(chan state, 1),
		nodeStatusChan:   make(chan struct{}, 1),
		dispatcher:       newDispatcher(),
	}
	h.setCallback("", errors.New("failing"))

	//
	// Initialisation and unknown state
	//

	ctx, cancelRun := context.WithCancel(context.Background())
	runReturned := make(chan struct{}, 1)
	go func() {
		h.Run(ctx)
		runReturned <- struct{}{}
	}()
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 250*time.Millisecond, func() bool {
		// State is unknown
		h.m.RLock()
		defer h.m.RUnlock()
		return h.state == unknown
	})
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 250*time.Millisecond, func() bool {
		// API replys not ready
		code, reason := h.ShouldHandle()
		return code == http.StatusServiceUnavailable && reason == notReadyReason
	})

	//
	// Unknown -> follower
	//

	h.setCallback("1.2.3.4", nil)
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 250*time.Millisecond, func() bool {
		// Internal state change
		h.m.RLock()
		defer h.m.RUnlock()
		return h.state == follower && h.leaderIP == "1.2.3.4"
	})
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 250*time.Millisecond, func() bool {
		// API redirects to leader
		code, reason := h.ShouldHandle()
		return code == http.StatusFound && reason == "1.2.3.4"
	})

	//
	// Follower -> leader
	//

	h.setCallback("", nil)
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 250*time.Millisecond, func() bool {
		// Internal state change
		h.m.RLock()
		defer h.m.RUnlock()
		return h.state == leader && h.leaderIP == ""
	})
	// We should not hook up to the AD yet
	assert.Len(t, ac.Calls, 0)
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 250*time.Millisecond, func() bool {
		// API serves requests
		code, reason := h.ShouldHandle()
		return code == http.StatusOK && reason == ""
	})

	// Trigger warmup and AC connection
	ac.On("AddScheduler", schedulerName, mock.AnythingOfType("*clusterchecks.dispatcher"), true).Return()
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 250*time.Millisecond, func() bool {
		// Keep node-agent caches even when timestamp is off (warmup)
		response, err := h.PostStatus("dummy", types.NodeStatus{LastChange: -50})
		return err == nil && response.IsUpToDate == true
	})
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 500*time.Millisecond, func() bool {
		// Test whether we're connected to the AD
		return ac.AssertNumberOfCalls(dummyT, "AddScheduler", 1)

	})
	ac.AssertExpectations(t)

	// Schedule a check and make sure it is assigned to the node "dummy"
	testConfig := integration.Config{
		Name:         "unit_test",
		ClusterCheck: true,
	}
	h.dispatcher.Schedule([]integration.Config{testConfig})
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 250*time.Millisecond, func() bool {
		// Found one configuration for node dummy
		configs, err := h.GetConfigs("dummy")
		return err == nil && len(configs.Configs) == 1
	})
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 250*time.Millisecond, func() bool {
		// Flush node-agent caches when timestamp is off
		response, err := h.PostStatus("dummy", types.NodeStatus{LastChange: -50})
		return err == nil && response.IsUpToDate == false
	})

	//
	// Leader -> follower
	//

	ac.On("RemoveScheduler", schedulerName).Return()
	h.setCallback("1.2.3.6", nil)
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 250*time.Millisecond, func() bool {
		// Internal state change
		h.m.RLock()
		defer h.m.RUnlock()
		return h.state == follower && h.leaderIP == "1.2.3.6"
	})
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 250*time.Millisecond, func() bool {
		// Dispatcher is flushed, no config remain
		allconfigs, err := h.GetAllConfigs()
		return err == nil && len(allconfigs.Configs) == 0
	})
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 250*time.Millisecond, func() bool {
		// API redirects to leader again
		code, reason := h.ShouldHandle()
		return code == http.StatusFound && reason == "1.2.3.6"
	})
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 500*time.Millisecond, func() bool {
		// RemoveScheduler is called
		return ac.AssertNumberOfCalls(dummyT, "RemoveScheduler", 1)
	})
	ac.AssertExpectations(t)

	//
	// Follower -> leader again
	//

	h.setCallback("", nil)
	h.PostStatus("dummy", types.NodeStatus{})
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 250*time.Millisecond, func() bool {
		// API serves requests
		code, reason := h.ShouldHandle()
		return code == http.StatusOK && reason == ""
	})
	h.PostStatus("dummy", types.NodeStatus{})
	assertTrueBeforeTimeout(t, 10*time.Millisecond, 500*time.Millisecond, func() bool {
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

	assertTrueBeforeTimeout(t, 10*time.Millisecond, 500*time.Millisecond, func() bool {
		// RemoveScheduler is called
		return ac.AssertNumberOfCalls(dummyT, "RemoveScheduler", 2)
	})
}
