// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildGlobalTagsMap(t *testing.T) {
	arn := "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	awsAccount := "123456789012"
	functionName := "my-function"
	region := "us-east-1"
	m := buildGlobalTagsMap(arn, functionName, region, awsAccount)
	assert.Equal(t, len(m), 6)
	assert.Equal(t, region, m["region"])
	assert.Equal(t, functionName, m["functionname"])
	assert.Equal(t, awsAccount, m["aws_account"])
	assert.Equal(t, arn, m["function_arn"])
	assert.Equal(t, "lambda", m["_dd.origin"])
	assert.Equal(t, "1", m["_dd.compute_stats"])
}
