// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package auditorimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuditorUnmarshalRegistryV3(t *testing.T) {
	input := `{
	    "Registry": {
	        "path1.log": {
	            "Offset": "12345",
	            "LastUpdated": "2006-01-12T01:01:01.000000001Z",
				"Fingerprint": 11111
	        },
	        "fingerprint:efgh": {
	            "Offset": "54321",
	            "LastUpdated": "2006-01-12T01:01:02.000000001Z",
				"Fingerprint": 22222
	        }
	    },
	    "Version": 3
	}`
	r, err := unmarshalRegistryV3([]byte(input))
	assert.Nil(t, err)

	assert.Equal(t, "12345", r["path1.log"].Offset)
	assert.Equal(t, 1, r["path1.log"].LastUpdated.Second())
	assert.Equal(t, uint64(11111), r["path1.log"].Fingerprint)

	assert.Equal(t, "54321", r["fingerprint:efgh"].Offset)
	assert.Equal(t, 2, r["fingerprint:efgh"].LastUpdated.Second())
	assert.Equal(t, uint64(22222), r["fingerprint:efgh"].Fingerprint)
}
