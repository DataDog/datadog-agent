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

//go:embed fixtures/apm_stats_bytes
var apmStatsData []byte

func TestAPMStatsAggregator(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		payloads, err := ParseAPMStatsPayload(api.Payload{Data: []byte{}})
		assert.Error(t, err)
		assert.Nil(t, payloads)
	})
	t.Run("malformed", func(t *testing.T) {
		payloads, err := ParseAPMStatsPayload(api.Payload{Data: []byte("not a payload")})
		assert.Error(t, err)
		assert.Nil(t, payloads)
	})
	t.Run("valid", func(t *testing.T) {
		payloads, err := ParseAPMStatsPayload(api.Payload{Data: apmStatsData, Encoding: encodingGzip})
		assert.NoError(t, err)
		assert.Len(t, payloads, 1)
		assert.Equal(t, "dev.host", payloads[0].AgentHostname)
		assert.Equal(t, "dev.env", payloads[0].AgentEnv)
		assert.Len(t, payloads[0].Stats, 1)
		assert.Len(t, payloads[0].Stats[0].Stats, 1)
		assert.Len(t, payloads[0].Stats[0].Stats[0].Stats, 1)
		assert.Equal(t, payloads[0].Stats[0].Stats[0].Stats[0].Name, "shell.operation")
		assert.Equal(t, payloads[0].Stats[0].Stats[0].Stats[0].Resource, "/my/bash/resource")
		assert.Equal(t, payloads[0].Stats[0].Stats[0].Stats[0].Service, "shell.service")
	})
}
