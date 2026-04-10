// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"strings"
	"testing"
	"time"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTagSetExcludesDualSendUUID(t *testing.T) {
	source := sources.NewLogSource("test", &logsconfig.LogsConfig{
		Tags: []string{"config:tag"},
	})
	origin := message.NewOrigin(source)
	origin.SetTags([]string{"origin:tag"})
	origin.SetService("service")
	origin.SetSource("source")

	msg := message.NewMessage([]byte("hello"), origin, "info", 0)
	msg.MessageMetadata.Hostname = "hostname"
	msg.MessageMetadata.ProcessingTags = []string{"processing:tag"}
	msg.MessageMetadata.DualSendUUID = "uuid-123"

	translator := NewMessageTranslator("test")
	tagSet, allTags, _, _ := translator.buildTagSet(msg)

	require.NotNil(t, tagSet)
	assert.Equal(t, "uuid-123", msg.MessageMetadata.DualSendUUID)
	assert.NotContains(t, allTags, "dual-send-uuid:")
	assert.True(t, strings.Contains(allTags, "origin:tag"))
	assert.True(t, strings.Contains(allTags, "config:tag"))
	assert.True(t, strings.Contains(allTags, "processing:tag"))
	assert.True(t, strings.Contains(allTags, "hostname:hostname"))
	assert.True(t, strings.Contains(allTags, "service:service"))
	assert.True(t, strings.Contains(allTags, "ddsource:source"))
	assert.False(t, strings.Contains(allTags, "status:"), "status should not be in tag set, it is sent separately")
}

func TestBuildStructuredLogSetsUUID(t *testing.T) {
	datum := buildStructuredLog(123, 456, nil, nil, "uuid-123", nil, 0, nil, nil)

	require.NotNil(t, datum.GetLogs())
	assert.Equal(t, "uuid-123", datum.GetLogs().GetUuid())
}

func TestBuildRawLogSetsUUID(t *testing.T) {
	datum := buildRawLog("hello", time.Unix(1, 0).UTC(), nil, "uuid-123")

	require.NotNil(t, datum.GetLogs())
	assert.Equal(t, "uuid-123", datum.GetLogs().GetUuid())
}
