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

//go:embed fixtures/metadata_bytes
var metadataBytes []byte

func TestMetadataAggregator(t *testing.T) {
	t.Run("parseMetadata empty JSON object should be ignored", func(t *testing.T) {
		metadata, err := ParseMetricSeries(api.Payload{
			Data:     []byte("{}"),
			Encoding: encodingJSON,
		})
		assert.NoError(t, err)
		assert.Empty(t, metadata)
	})

	t.Run("parseMetadata valid body should parse metadata", func(t *testing.T) {
		metadata, err := ParseMetadataPayload(api.Payload{Data: metadataBytes, Encoding: encodingDeflate})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(metadata))
		assert.Equal(t, "i-0473fb6c2bd4591b4", metadata[0].Hostname)
		assert.Equal(t, int64(1707399160845285933), metadata[0].Timestamp)
		assert.Len(t, metadata[0].Metadata, 66)
	})
}
