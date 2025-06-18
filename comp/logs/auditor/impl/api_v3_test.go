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
	        "fingerprint:abcd": {
	            "Offset": "12345",
	            "LastUpdated": "2006-01-12T01:01:01.000000001Z",
				"FilePath": "/var/log/path1.log"
	        },
	        "fingerprint:efgh": {
	            "Offset": "54321",
	            "LastUpdated": "2006-01-12T01:01:02.000000001Z",
				"FilePath": "/var/log/path2.log"
	        }
	    },
	    "Version": 3
	}`
	r, err := unmarshalRegistryV3([]byte(input))
	assert.Nil(t, err)

	assert.Equal(t, "12345", r["fingerprint:abcd"].Offset)
	assert.Equal(t, 1, r["fingerprint:abcd"].LastUpdated.Second())
	assert.Equal(t, "/var/log/path1.log", r["fingerprint:abcd"].FilePath)

	assert.Equal(t, "54321", r["fingerprint:efgh"].Offset)
	assert.Equal(t, 2, r["fingerprint:efgh"].LastUpdated.Second())
	assert.Equal(t, "/var/log/path2.log", r["fingerprint:efgh"].FilePath)
}
