// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldHandle(t *testing.T) {
	h := &Handler{
		port: 5005,
	}

	// Initial state
	code, reason := h.ShouldHandle()
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, notReadyReason, reason)

	// Leader and ready
	h.state = leader
	code, reason = h.ShouldHandle()
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "", reason)

	// Follower
	h.state = follower
	h.leaderIP = "1.2.3.4"
	code, reason = h.ShouldHandle()
	assert.Equal(t, http.StatusFound, code)
	assert.Equal(t, "1.2.3.4:5005", reason)
}
