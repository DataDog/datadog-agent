// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auditor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuditorUnmarshalRegistryV0(t *testing.T) {
	input := `{
	    "Registry": {
	        "path1.log": {
	            "Offset": 1,
	            "Path": "path1.log",
	            "Timestamp": "2006-01-12T01:01:01.000000001Z"
	        },
	        "path2.log": {
	            "Offset": 2,
	            "Path": "path2.log",
	            "Timestamp": "2006-01-12T01:01:02.000000001Z"
	        }
	    },
	    "Version": 0
	}`
	r, err := unmarshalRegistryV0([]byte(input))
	assert.Nil(t, err)
	assert.Equal(t, "1", r["file:path1.log"].Offset)
	assert.Equal(t, 1, r["file:path1.log"].LastUpdated.Second())
	assert.Equal(t, "2", r["file:path2.log"].Offset)
	assert.Equal(t, 2, r["file:path2.log"].LastUpdated.Second(), 2)
}
