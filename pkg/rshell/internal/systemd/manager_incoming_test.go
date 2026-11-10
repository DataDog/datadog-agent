// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
)

type countingManagerCloser struct {
	closes int
}

func (c *countingManagerCloser) Close() error {
	c.closes++
	return nil
}

func TestRejectManagerInboundMethodCallsClosesOnce(t *testing.T) {
	transport := &countingManagerCloser{}
	intercept := rejectManagerInboundMethodCalls(transport)
	intercept(nil)
	intercept(&dbus.Message{Type: dbus.TypeSignal})
	intercept(&dbus.Message{Type: dbus.TypeMethodReply})
	assert.Zero(t, transport.closes)

	intercept(&dbus.Message{Type: dbus.TypeMethodCall})
	intercept(&dbus.Message{Type: dbus.TypeMethodCall})
	assert.Equal(t, 1, transport.closes)
}
