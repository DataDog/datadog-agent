// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auditor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuditorUnmarshalRegistryV1(t *testing.T) {
	input := `{
	    "Registry": {
	        "path1.log": {
	            "Offset": 1,
	            "LastUpdated": "2006-01-12T01:01:01.000000001Z",
	            "Timestamp": ""
	        },
	        "path2.log": {
	            "Offset": 0,
	            "LastUpdated": "2006-01-12T01:01:02.000000001Z",
	            "Timestamp": "2006-01-12T01:01:03.000000001Z"
	        }
	    },
	    "Version": 1
	}`
	r, err := unmarshalRegistryV1([]byte(input))
	assert.Nil(t, err)
	assert.Equal(t, "1", r["path1.log"].Offset)
	assert.Equal(t, 1, r["path1.log"].LastUpdated.Second())

	assert.Equal(t, "2006-01-12T01:01:03.000000001Z", r["path2.log"].Offset)
	assert.Equal(t, 2, r["path2.log"].LastUpdated.Second(), 2)
}
