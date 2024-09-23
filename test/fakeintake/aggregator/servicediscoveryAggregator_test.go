// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

//go:embed fixtures/servicediscovery_bytes
var serviceDiscoveryData []byte

func TestServiceDiscoveryAggregator(t *testing.T) {
	t.Run("empty payload", func(t *testing.T) {
		payloads, err := ParseServiceDiscoveryPayload(api.Payload{Data: []byte("")})
		assert.Error(t, err)
		assert.Nil(t, payloads)
	})

	t.Run("malformed payload", func(t *testing.T) {
		payloads, err := ParseServiceDiscoveryPayload(api.Payload{Data: []byte("not a payload")})
		assert.Error(t, err)
		assert.Nil(t, payloads)
	})

	t.Run("valid payload", func(t *testing.T) {
		payloads, err := ParseServiceDiscoveryPayload(api.Payload{Data: serviceDiscoveryData, Encoding: encodingGzip})
		require.NoError(t, err)
		require.Len(t, payloads, 3)

		assert.Equal(t, "start-service", payloads[0].RequestType)
		assert.Equal(t, "chronyd", payloads[0].Payload.ServiceName)
		assert.Equal(t, "web_service", payloads[1].Payload.ServiceType)
		assert.Equal(t, "ip-10-1-60-129", payloads[2].Payload.HostName)
	})
}
