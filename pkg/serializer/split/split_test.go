// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package split

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
)

type rawPayload []byte

func (p rawPayload) MarshalJSON() ([]byte, error) { return p, nil }

// fixedCompressor "compresses" any payload into compressedSize bytes.
type fixedCompressor struct {
	compression.Component
	compressedSize int
}

func (c fixedCompressor) Compress(_ []byte) ([]byte, error) {
	return make([]byte, c.compressedSize), nil
}

func TestCheckSizeAndSerialize(t *testing.T) {
	origCompressed, origUncompressed := maxPayloadSizeCompressed, maxPayloadSizeUnCompressed
	t.Cleanup(func() { maxPayloadSizeCompressed, maxPayloadSizeUnCompressed = origCompressed, origUncompressed })
	maxPayloadSizeCompressed = 10
	maxPayloadSizeUnCompressed = 100

	tests := []struct {
		name             string
		uncompressedSize int
		compressedSize   int
		expected         SizeLimit
	}{
		{name: "within limits", uncompressedSize: 100, compressedSize: 10, expected: WithinLimits},
		{name: "compressed too big", uncompressedSize: 100, compressedSize: 11, expected: CompressedLimit},
		{name: "uncompressed too big", uncompressedSize: 101, compressedSize: 10, expected: UncompressedLimit},
		{name: "both too big", uncompressedSize: 101, compressedSize: 11, expected: CompressedLimit},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := rawPayload(make([]byte, tc.uncompressedSize))
			exceeded, compressed, uncompressed, err := CheckSizeAndSerialize(payload, true, fixedCompressor{compressedSize: tc.compressedSize})
			require.NoError(t, err)
			assert.Equal(t, tc.expected, exceeded)
			assert.Len(t, compressed, tc.compressedSize)
			assert.Len(t, uncompressed, tc.uncompressedSize)
		})
	}
}
