// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
)

//go:embed fixtures/trace_bytes
var traceData []byte

func TestTraceAggregator(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		payloads, err := ParseTracePayload(api.Payload{Data: []byte{}})
		assert.NoError(t, err)
		assert.Empty(t, payloads[0].TracerPayloads)
	})
	t.Run("malformed", func(t *testing.T) {
		payloads, err := ParseTracePayload(api.Payload{Data: []byte("not a payload")})
		assert.Error(t, err)
		assert.Nil(t, payloads)
	})
	t.Run("valid", func(t *testing.T) {
		payloads, err := ParseTracePayload(api.Payload{Data: traceData, Encoding: encodingGzip})
		assert.NoError(t, err)
		assert.Len(t, payloads, 1)
		assert.Equal(t, "dev.host", payloads[0].HostName)
		assert.Equal(t, "dev.env", payloads[0].Env)
		assert.Len(t, payloads[0].TracerPayloads, 1)
		assert.Equal(t, payloads[0].TracerPayloads[0].LanguageName, "shell")
		assert.Len(t, payloads[0].TracerPayloads[0].Chunks, 1)
		assert.Len(t, payloads[0].TracerPayloads[0].Chunks[0].Spans, 1)
		assert.Equal(t, payloads[0].TracerPayloads[0].Chunks[0].Spans[0].Name, "shell.operation")
		assert.Equal(t, payloads[0].TracerPayloads[0].Chunks[0].Spans[0].Resource, "/my/bash/resource")
	})
}
