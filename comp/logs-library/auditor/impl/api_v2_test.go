// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package auditorimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditorUnmarshalRegistryV2(t *testing.T) {
	input := `{
			"Registry": {
				"path1.log": {
					"Offset": "12345",
					"LastUpdated": "2006-01-12T01:01:01.000000001Z",
					"Fingerprint": {
						"Value": 11111,
						"Config": {
							"fingerprint_strategy": "line_checksum",
							"count": 200,
							"max_bytes": 1024,
							"count_to_skip": 5
						}
					}
				}
			},
			"Version": 2
		}`
	r, err := unmarshalRegistryV2([]byte(input))
	require.NoError(t, err)

	entry, exists := r["path1.log"]
	require.True(t, exists)

	assert.Equal(t, "12345", entry.Offset)
	assert.Equal(t, 1, entry.LastUpdated.Second())
	assert.Equal(t, uint64(11111), entry.Fingerprint.Value)
	require.NotNil(t, entry.Fingerprint.Config)
	assert.Equal(t, "line_checksum", string(entry.Fingerprint.Config.FingerprintStrategy))
	assert.Equal(t, 200, entry.Fingerprint.Config.Count)
	assert.Equal(t, 1024, entry.Fingerprint.Config.MaxBytes)
	assert.Equal(t, 5, entry.Fingerprint.Config.CountToSkip)
}
