// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package module

import (
	"testing"

	"github.com/go-openapi/testify/v2/require"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usersessions"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
)

// TestPendingMsgIsResolvedSSHSession makes sure that an event of an SSH session waits for its
// authentication log line to be tailed, but only for a bounded number of retries, and only once per
// session: the retry queue is ordered, so waiting longer would delay all the following events.
func TestPendingMsgIsResolvedSSHSession(t *testing.T) {
	resolver, err := usersessions.NewResolver(64, true)
	require.NoError(t, err)

	newPatcher := func() sshSessionPatcher {
		return probe.NewSSHUserSessionPatcher(
			&serializers.SSHSessionContextSerializer{
				SSHClientIP:   "127.0.0.1",
				SSHClientPort: 38835,
			},
			resolver,
			4242,
		)
	}
	newMsg := func(retry int) *pendingMsg {
		return &pendingMsg{
			ruleID:            "test-rule",
			retry:             retry,
			sshSessionPatcher: newPatcher(),
		}
	}
	maxRetry := newPatcher().MaxRetry()

	// the auth log line has not been tailed yet, the event must be held back
	assert.False(t, newMsg(0).isResolved(), "event must wait for the ssh session to be resolved")
	assert.False(t, newMsg(maxRetry-1).isResolved(), "event must wait until its retry budget is exhausted")

	// budget exhausted: the event is sent as is, and the session is flagged so that the next events
	// of the same session are not delayed anymore
	key := usersessions.SSHSessionKey{SSHDPid: "4242", IP: "127.0.0.1", Port: "38835"}
	assert.False(t, resolver.IsSSHSessionUnresolved(key))
	assert.True(t, newMsg(maxRetry).isResolved(), "event must be sent once the retry budget is exhausted")
	assert.True(t, resolver.IsSSHSessionUnresolved(key), "the ssh session must be flagged as unresolved")

	// following events of that session must not be delayed
	assert.True(t, newMsg(0).isResolved(), "events of an unresolved session must not be delayed")
}
