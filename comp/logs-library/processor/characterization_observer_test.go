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

	observer := newCharacterizationObserver()
	observation := makeCharacterizationObservation(msg, "2", observer.sourceSeed)
	require.Equal(t, 5, observation.contentBytes)
	require.Equal(t, 7, observation.rawBytes)
	require.Equal(t, 2, observation.tagCount)
	require.Equal(t, len("env:test,parser:tag"), observation.tagBytes)
	require.Equal(t, config.FileType, observation.sourceType)
	require.Equal(t, "2", observation.pipeline)
	require.Equal(t, "plain", observation.payloadFamily)
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

func TestCharacterizationSourceCardinalityIsBoundedAndDoesNotRetainIdentifiers(t *testing.T) {
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.FileType})
	observer := newCharacterizationObserver()
	first := message.NewMessageWithSource([]byte("first"), "info", source, 0)
	first.Origin.Identifier = "/private/first.log"
	second := message.NewMessageWithSource([]byte("second"), "info", source, 0)
	second.Origin.Identifier = "/private/second.log"

	firstObservation := makeCharacterizationObservation(first, "1", observer.sourceSeed)
	secondObservation := makeCharacterizationObservation(second, "1", observer.sourceSeed)
	require.NotZero(t, firstObservation.sourceHash)
	require.NotEqual(t, firstObservation.sourceHash, secondObservation.sourceHash)

	observer.recordSource(firstObservation)
	observer.recordSource(firstObservation)
	observer.recordSource(secondObservation)
	require.Len(t, observer.sourceIDs, 2)
	require.Equal(t, 2, observer.sourceCounts[config.FileType])

	observer.sourceIDs = make(map[characterizationSourceIdentity]struct{})
	for index := 0; index < characterizationSourceLimit; index++ {
		observer.sourceIDs[characterizationSourceIdentity{sourceType: config.FileType, hash: uint64(index + 1)}] = struct{}{}
	}
	observer.recordSource(characterizationObservation{
		sourceType: config.FileType, pipeline: "1", sourceHash: ^uint64(0), hasSourceID: true,
	})
	require.Len(t, observer.sourceIDs, characterizationSourceLimit)
}

func TestCharacterizationPayloadFamilyIsAllowlistedAndScanBounded(t *testing.T) {
	tests := map[string]string{
		"":                                       "empty",
		"hello":                                  "plain",
		` {"id": 1}`:                             "json",
		`[{"id": 1}]`:                            "json",
		`{"message":"hello","ddsource":"nginx"}`: "datadog_json",
		`127.0.0.1 - - [date] "GET / HTTP/1.1" 200 1`: "apache_common",
	}
	for content, expected := range tests {
		require.Equal(t, expected, characterizationPayloadFamily([]byte(content)))
	}
	content := append(make([]byte, characterizationScanLimit), []byte(`{"message":"hidden","ddsource":"hidden"}`)...)
	require.Equal(t, "plain", characterizationPayloadFamily(content))
}
