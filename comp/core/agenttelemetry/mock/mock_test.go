// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package mock

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendEvent_RegisteredEventValidPayload_Recorded(t *testing.T) {
	m := New(t, "agentbsod")

	err := m.SendEvent("agentbsod", []byte(`{"offender":"ddnpm+0x1a3"}`))
	require.NoError(t, err)

	events := m.Events()
	require.Len(t, events, 1)
	assert.Equal(t, "agentbsod", events[0].Type)
	assert.JSONEq(t, `{"offender":"ddnpm+0x1a3"}`, string(events[0].Payload))
}

func TestSendEvent_UnregisteredEvent_Errors(t *testing.T) {
	m := New(t, "agentbsod")

	err := m.SendEvent("not-registered", []byte(`{}`))
	require.Error(t, err)
	assert.Empty(t, m.Events())
}

func TestSendEvent_NoRegisteredEvents_RejectsEverything(t *testing.T) {
	m := New(t)

	err := m.SendEvent("agentbsod", []byte(`{}`))
	require.Error(t, err)
	assert.Empty(t, m.Events())
}

func TestSendEvent_InvalidJSONPayload_Errors(t *testing.T) {
	m := New(t, "agentbsod")

	err := m.SendEvent("agentbsod", []byte(`not-json`))
	require.Error(t, err)
	assert.Empty(t, m.Events())
}

func TestSubmitErrorLog_Recorded(t *testing.T) {
	m := New(t)

	log := errortracking.ErrorLog{Count: 1, ErrorKind: "*net.OpError"}
	m.SubmitErrorLog(log)

	assert.Equal(t, []errortracking.ErrorLog{log}, m.ErrorLogs())
}
