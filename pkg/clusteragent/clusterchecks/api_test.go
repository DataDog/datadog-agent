// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

func TestShouldHandle(t *testing.T) {
	h := &Handler{}

	// Initial state
	code, reason := h.ShouldHandle()
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, notReadyReason, reason)

	// Leader and ready
	h.state = leader
	h.dispatcher = newDispatcher()
	code, reason = h.ShouldHandle()
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "", reason)

	// Follower with an active dispatcher (ongoing stop)
	h.state = follower
	h.leaderIP = "1.2.3.4"
	h.dispatcher = newDispatcher()
	code, reason = h.ShouldHandle()
	assert.Equal(t, http.StatusFound, code)
	assert.Equal(t, "1.2.3.4", reason)

	// Follower
	h.leaderIP = "1.2.3.4"
	h.dispatcher = nil
	code, reason = h.ShouldHandle()
	assert.Equal(t, http.StatusFound, code)
	assert.Equal(t, "1.2.3.4", reason)
}

func TestPostStatusNonBlocking(t *testing.T) {
	h := &Handler{
		dispatcher:     newDispatcher(),
		nodeStatusChan: make(chan struct{}, 1),
	}

	nodes := []string{"a", "b", "c", "d"}

	for _, n := range nodes {
		// Run the health status in a goroutine
		ch := make(chan struct{}, 1)
		go func() {
			h.PostStatus(n, types.NodeStatus{})
			ch <- struct{}{}
		}()

		select {
		case <-ch:
			break
		case <-time.After(50 * time.Millisecond):
			assert.Fail(t, fmt.Sprintf("Status for node %q was blocking", n))
		}
	}

	select {
	case <-h.nodeStatusChan:
		break
	case <-time.After(50 * time.Millisecond):
		assert.Fail(t, "Timeout while waiting for channel message")
	}
}
