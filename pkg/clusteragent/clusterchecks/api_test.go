// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
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
