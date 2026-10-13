// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserSessions(t *testing.T) {
	// The user sessions count is collected even when the process memory and
	// inactive sessions collectors are disabled.
	c, s := newDefaultCheck(t, `
process_memory:
  enabled: false
inactive_sessions:
  enabled: false
user_sessions_count:
  enabled: true`, "")
	err := c.Run()
	require.NoError(t, err)
	s.AssertMetricTaggedWith(t, "Gauge", userSessionsMetricName, []string{})
}

func TestUserSessionsDisabled(t *testing.T) {
	c, s := newDefaultCheck(t, `
user_sessions_count:
  enabled: false`, "")
	err := c.Run()
	require.NoError(t, err)
	s.AssertMetricMissing(t, "Gauge", userSessionsMetricName)
}
