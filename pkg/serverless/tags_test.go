// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetIfNotEmptyWithNonEmptyKey(t *testing.T) {
	testMap := make(map[string]string)
	testMap = setIfNotEmpty(testMap, "nonEmptyKey", "VALUE")
	assert.Equal(t, 1, len(testMap))
	assert.Equal(t, "value", testMap["nonEmptyKey"])
}

func TestSetIfNotEmptyWithEmptyKey(t *testing.T) {
	testMap := make(map[string]string)
	testMap = setIfNotEmpty(testMap, "", "VALUE")
	assert.Equal(t, 0, len(testMap))
}

func TestBuildTracerTags(t *testing.T) {
	tagsMap := map[string]string{
		"key0":     "value0",
		"resource": "value1",
		"key1":     "value1",
	}
	resultTagsMap := buildTracerTags(tagsMap)
	assert.Equal(t, 2, len(resultTagsMap))
	assert.Equal(t, "value0", resultTagsMap["key0"])
	assert.Equal(t, "value1", resultTagsMap["key1"])
}

func TestBuildTagsFromMap(t *testing.T) {
	tagsMap := map[string]string{
		"key0":              "value0",
		"key1":              "value1",
		"key2":              "value2",
		"key3":              "value3",
		"_dd.origin":        "xxx",
		"_dd.compute_stats": "xxx",
	}
	configTags := []string{"configTagKey0:configTagValue0", "configTagKey1:configTagValue1"}
	resultTagsArray := buildTagsFromMap(configTags, tagsMap)
	sort.Strings(resultTagsArray)
	assert.Equal(t, []string{
		"configTagKey0:configTagValue0",
		"configTagKey1:configTagValue1",
		"key0:value0",
		"key1:value1",
		"key2:value2",
		"key3:value3",
	}, resultTagsArray)
}

func TestBuildTagMapFromArnIncomplete(t *testing.T) {
	arn := "function:my-function"
	tagMap := buildTagMapFromArn(arn)
	assert.Equal(t, 3, len(tagMap))
	assert.Equal(t, "lambda", tagMap["_dd.origin"])
	assert.Equal(t, "1", tagMap["_dd.compute_stats"])
	assert.Equal(t, "function:my-function", tagMap["function_arn"])
}

func TestBuildTagMapFromArnComplete(t *testing.T) {
	arn := "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	tagMap := buildTagMapFromArn(arn)
	assert.Equal(t, 8, len(tagMap))
	assert.Equal(t, "lambda", tagMap["_dd.origin"])
	assert.Equal(t, "1", tagMap["_dd.compute_stats"])
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:my-function", tagMap["function_arn"])
	assert.Equal(t, "us-east-1", tagMap["region"])
	assert.Equal(t, "123456789012", tagMap["aws_account"])
	assert.Equal(t, "123456789012", tagMap["account_id"])
	assert.Equal(t, "my-function", tagMap["functionname"])
	assert.Equal(t, "my-function", tagMap["resource"])
}

func TestBuildTagMapFromArnCompleteWithUpperCase(t *testing.T) {
	arn := "arn:aws:lambda:us-east-1:123456789012:function:My-Function"
	tagMap := buildTagMapFromArn(arn)
	assert.Equal(t, 8, len(tagMap))
	assert.Equal(t, "lambda", tagMap["_dd.origin"])
	assert.Equal(t, "1", tagMap["_dd.compute_stats"])
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:my-function", tagMap["function_arn"])
	assert.Equal(t, "us-east-1", tagMap["region"])
	assert.Equal(t, "123456789012", tagMap["aws_account"])
	assert.Equal(t, "123456789012", tagMap["account_id"])
	assert.Equal(t, "my-function", tagMap["functionname"])
	assert.Equal(t, "my-function", tagMap["resource"])
}

func TestBuildTagMapFromArnCompleteWithLatest(t *testing.T) {
	os.Setenv("AWS_LAMBDA_FUNCTION_VERSION", "$LATEST")
	arn := "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	tagMap := buildTagMapFromArn(arn)
	assert.Equal(t, 8, len(tagMap))
	assert.Equal(t, "lambda", tagMap["_dd.origin"])
	assert.Equal(t, "1", tagMap["_dd.compute_stats"])
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:my-function", tagMap["function_arn"])
	assert.Equal(t, "us-east-1", tagMap["region"])
	assert.Equal(t, "123456789012", tagMap["aws_account"])
	assert.Equal(t, "123456789012", tagMap["account_id"])
	assert.Equal(t, "my-function", tagMap["functionname"])
	assert.Equal(t, "my-function", tagMap["resource"])
}

func TestBuildTagMapFromArnCompleteWithVersionNumber(t *testing.T) {
	os.Setenv("AWS_LAMBDA_FUNCTION_VERSION", "888")
	arn := "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	tagMap := buildTagMapFromArn(arn)
	assert.Equal(t, 9, len(tagMap))
	assert.Equal(t, "lambda", tagMap["_dd.origin"])
	assert.Equal(t, "1", tagMap["_dd.compute_stats"])
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:my-function", tagMap["function_arn"])
	assert.Equal(t, "us-east-1", tagMap["region"])
	assert.Equal(t, "123456789012", tagMap["aws_account"])
	assert.Equal(t, "123456789012", tagMap["account_id"])
	assert.Equal(t, "my-function", tagMap["functionname"])
	assert.Equal(t, "my-function:888", tagMap["resource"])
	assert.Equal(t, "888", tagMap["executedversion"])
}
