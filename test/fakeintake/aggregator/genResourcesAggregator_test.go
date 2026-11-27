// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	"testing"
	"time"

	"github.com/DataDog/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

//go:embed fixtures/genresources_bytes
var genResourcesFixture []byte

func TestGenResourcesAggregator(t *testing.T) {
	compressed, err := zstd.Compress(nil, genResourcesFixture)
	require.NoError(t, err)

	collectedTime := time.Unix(3, 0).UTC()
	agg := NewGenResourcesAggregator()
	require.NoError(t, agg.UnmarshallPayloads([]api.Payload{
		{Data: []byte("{}"), Encoding: encodingJSON},
		{Data: compressed, Encoding: encodingZstd, Timestamp: collectedTime},
	}))
	require.Equal(t, []string{"gpu_device:gpu-test"}, agg.GetNames())
	payloads := agg.GetPayloadsByName("gpu_device:gpu-test")
	require.Len(t, payloads, 1)
	payload := payloads[0]
	assert.Equal(t, collectedTime, payload.GetCollectedTime())
	assert.Equal(t, "integrations-core", payload.GetSource())
	assert.Equal(t, int64(1), payload.GetResource().GetSeenAt().GetSeconds())
	assert.Equal(t, int64(2), payload.GetResource().GetExpireAt().GetSeconds())
	assert.JSONEq(t, `{"hostname":"gpu-host","gpu_uuid":"gpu-test"}`, string(payload.GetResource().GetFieldsJson()))
}

func TestParseGenResourcesInvalidPayload(t *testing.T) {
	_, err := ParseGenResources(api.Payload{Data: []byte{0xff}})
	require.Error(t, err)
	_, err = ParseGenResources(api.Payload{Data: []byte{0xff}, Encoding: encodingZstd})
	require.Error(t, err)
}
