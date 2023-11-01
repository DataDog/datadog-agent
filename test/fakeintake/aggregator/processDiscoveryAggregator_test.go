// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/fakeintake/fixtures"
)

func TestProcessDiscoveryAggregator(t *testing.T) {
	payload := fixtures.CollectorProcDiscoveryPayload(t)

	t.Run("empty payload", func(t *testing.T) {
		payloads, err := ParseProcessDiscoveryPayload(api.Payload{Data: []byte("")})
		assert.Error(t, err)
		assert.Nil(t, payloads)
	})

	t.Run("malformed payload", func(t *testing.T) {
		payloads, err := ParseProcessDiscoveryPayload(api.Payload{Data: []byte("not a payload")})
		assert.Error(t, err)
		assert.Nil(t, payloads)
	})

	t.Run("valid payload", func(t *testing.T) {
		payloads, err := ParseProcessDiscoveryPayload(api.Payload{Data: payload})
		require.NoError(t, err)
		require.Len(t, payloads, 1)

		payload := payloads[0]
		assert.Equal(t, "i-078e212", payload.HostName)
		assert.Len(t, payload.ProcessDiscoveries, 6)
	})
}
