// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type encodedLog struct {
	Message  string `json:"message"`
	Hostname string `json:"hostname"`
	Status   string `json:"status"`
}

func TestSyncProcessorProcess(t *testing.T) {
	hostname, _ := hostnameinterface.NewMock("agent-host")
	p := NewSyncProcessor([]*config.ProcessingRule{
		newProcessingRule(config.ExcludeAtMatch, "", "drop me"),
		newProcessingRule(config.MaskSequences, "secret=[masked]", `secret=\w+`),
	}, JSONEncoder, hostname)
	source := sources.NewLogSource("", &config.LogsConfig{})

	t.Run("filtered out by a processing rule", func(t *testing.T) {
		send, err := p.Process(newMessage([]byte("please drop me"), source, message.StatusInfo))

		require.NoError(t, err)
		assert.False(t, send)
	})

	t.Run("processed and encoded", func(t *testing.T) {
		msg := newMessage([]byte("login secret=hunter2"), source, message.StatusWarning)

		send, err := p.Process(msg)

		require.NoError(t, err)
		require.True(t, send)
		var encoded encodedLog
		require.NoError(t, json.Unmarshal(msg.GetContent(), &encoded))
		assert.Equal(t, encodedLog{Message: "login secret=[masked]", Hostname: "agent-host", Status: message.StatusWarning}, encoded)
	})

	t.Run("message hostname takes precedence", func(t *testing.T) {
		msg := newMessage([]byte("hello"), source, message.StatusInfo)
		msg.Hostname = "log-host"

		send, err := p.Process(msg)

		require.NoError(t, err)
		require.True(t, send)
		var encoded encodedLog
		require.NoError(t, json.Unmarshal(msg.GetContent(), &encoded))
		assert.Equal(t, "log-host", encoded.Hostname)
	})

	t.Run("rules of the message source apply", func(t *testing.T) {
		withRule := newSource(config.ExcludeAtMatch, "", "noisy")

		send, err := p.Process(newMessage([]byte("noisy line"), &withRule, message.StatusInfo))

		require.NoError(t, err)
		assert.False(t, send)
	})
}
