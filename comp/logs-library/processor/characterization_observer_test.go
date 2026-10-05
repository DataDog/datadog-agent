// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestMakeCharacterizationObservation(t *testing.T) {
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:    config.FileType,
		Service: "service",
		Source:  "source",
		Tags:    []string{"env:test"},
	})
	msg := message.NewMessageWithSource([]byte("hello"), "info", source, 0)
	msg.RawDataLen = 7
	msg.ParsingExtra.Tags = []string{"parser:tag"}

	observation := makeCharacterizationObservation(msg, "2")
	require.Equal(t, 5, observation.contentBytes)
	require.Equal(t, 7, observation.rawBytes)
	require.Equal(t, 2, observation.tagCount)
	require.Equal(t, len("env:test,parser:tag"), observation.tagBytes)
	require.Equal(t, config.FileType, observation.sourceType)
	require.Equal(t, "2", observation.pipeline)
	require.True(t, observation.hasService)
	require.True(t, observation.hasSource)
}

func TestCharacterizationObserverQueueIsBoundedAndNonBlocking(t *testing.T) {
	observer := newCharacterizationObserver()
	msg := message.NewMessage([]byte("hello"), nil, "info", 0)

	for range characterizationQueueSize + 1 {
		observer.observe(msg, "0")
	}

	require.Len(t, observer.queue, characterizationQueueSize)
}

func TestCharacterizationSourceTypeIsAllowlisted(t *testing.T) {
	source := sources.NewLogSource("test", &config.LogsConfig{Type: "secret-custom-type"})
	msg := message.NewMessageWithSource([]byte("hello"), "info", source, 0)
	require.Equal(t, "unknown", characterizationSourceType(msg))
}
