// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
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

func TestSetIfNotEmptyWithEmptyValue(t *testing.T) {
	testMap := make(map[string]string)
	testMap = setIfNotEmpty(testMap, "nonEmptyKey", "")
	assert.Equal(t, 0, len(testMap))
}

func TestBuildTracerTags(t *testing.T) {
	tagsMap := map[string]string{
		"key0":     "value0",
		"resource": "value1",
		"key1":     "value1",
	}
	resultTagsMap := BuildTracerTags(tagsMap)
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
	resultTagsArray := BuildTagsFromMap(tagsMap)
	sort.Strings(resultTagsArray)
	assert.Equal(t, []string{
		"key0:value0",
		"key1:value1",
		"key2:value2",
		"key3:value3",
	}, resultTagsArray)
}

func TestBuildTagMapFromArnIncomplete(t *testing.T) {
	arn := "function:my-function"
	tagMap := BuildTagMap(arn, []string{"tag0:value0", "TAG1:VALUE1"})
	assert.Equal(t, 8, len(tagMap))
	assert.Equal(t, "lambda", tagMap["_dd.origin"])
	assert.Equal(t, "1", tagMap["_dd.compute_stats"])
	assert.Equal(t, "function:my-function", tagMap["function_arn"])
	assert.Equal(t, "xxx", tagMap["dd_extension_version"])
	assert.Equal(t, "value0", tagMap["tag0"])
	assert.Equal(t, "value1", tagMap["tag1"])
	// Result of this test depends on build environment
	assert.True(t, tagMap["architecture"] == "x86_64" || tagMap["architecture"] == "arm64")
	assert.True(t, tagMap["runtime"] == "unknown" || tagMap["runtime"] == "provided.al2")
}

func TestBuildTagMapFromArnIncompleteWithCommaAndSpaceTags(t *testing.T) {
	arn := "function:my-function"
	tagMap := BuildTagMap(arn, []string{"tag0:value0", "tag1:value1,tag2:VALUE2", "TAG3:VALUE3"})
	assert.Equal(t, 10, len(tagMap))
	assert.Equal(t, "lambda", tagMap["_dd.origin"])
	assert.Equal(t, "1", tagMap["_dd.compute_stats"])
	assert.Equal(t, "function:my-function", tagMap["function_arn"])
	assert.Equal(t, "xxx", tagMap["dd_extension_version"])
	assert.Equal(t, "value0", tagMap["tag0"])
	assert.Equal(t, "value1", tagMap["tag1"])
	assert.Equal(t, "value2", tagMap["tag2"])
	assert.Equal(t, "value3", tagMap["tag3"])
	// Result of this test depends on build environment
	assert.True(t, tagMap["architecture"] == "x86_64" || tagMap["architecture"] == "arm64")
	assert.True(t, tagMap["runtime"] == "unknown" || tagMap["runtime"] == "provided.al2")
}

func TestBuildTagMapFromArnComplete(t *testing.T) {
	arn := "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	tagMap := BuildTagMap(arn, []string{"tag0:value0", "TAG1:VALUE1"})
	assert.Equal(t, 13, len(tagMap))
	assert.Equal(t, "lambda", tagMap["_dd.origin"])
	assert.Equal(t, "1", tagMap["_dd.compute_stats"])
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:my-function", tagMap["function_arn"])
	assert.Equal(t, "us-east-1", tagMap["region"])
	assert.Equal(t, "123456789012", tagMap["aws_account"])
	assert.Equal(t, "123456789012", tagMap["account_id"])
	assert.Equal(t, "my-function", tagMap["functionname"])
	assert.Equal(t, "my-function", tagMap["resource"])
	assert.Equal(t, "xxx", tagMap["dd_extension_version"])
	assert.Equal(t, "value0", tagMap["tag0"])
	assert.Equal(t, "value1", tagMap["tag1"])
	// Result of this test depends on build environment
	assert.True(t, tagMap["architecture"] == "x86_64" || tagMap["architecture"] == "arm64")
	assert.True(t, tagMap["runtime"] == "unknown" || tagMap["runtime"] == "provided.al2")
}

func TestBuildTagMapFromArnCompleteWithEnvAndVersionAndService(t *testing.T) {
	t.Setenv("DD_VERSION", "myTestVersion")
	t.Setenv("DD_ENV", "myTestEnv")
	t.Setenv("DD_SERVICE", "myTestService")

	arn := "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	tagMap := BuildTagMap(arn, []string{"tag0:value0", "TAG1:VALUE1"})
	assert.Equal(t, 16, len(tagMap))
	assert.Equal(t, "mytestenv", tagMap["env"])
	assert.Equal(t, "mytestversion", tagMap["version"])
	assert.Equal(t, "mytestservice", tagMap["service"])
	assert.Equal(t, "lambda", tagMap["_dd.origin"])
	assert.Equal(t, "1", tagMap["_dd.compute_stats"])
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:my-function", tagMap["function_arn"])
	assert.Equal(t, "us-east-1", tagMap["region"])
	assert.Equal(t, "123456789012", tagMap["aws_account"])
	assert.Equal(t, "123456789012", tagMap["account_id"])
	assert.Equal(t, "my-function", tagMap["functionname"])
	assert.Equal(t, "my-function", tagMap["resource"])
	assert.Equal(t, "xxx", tagMap["dd_extension_version"])
	assert.Equal(t, "value0", tagMap["tag0"])
	assert.Equal(t, "value1", tagMap["tag1"])
	// Result of this test depends on build environment
	assert.True(t, tagMap["architecture"] == "x86_64" || tagMap["architecture"] == "arm64")
	assert.True(t, tagMap["runtime"] == "unknown" || tagMap["runtime"] == "provided.al2")
}

func TestBuildTagMapFromArnCompleteWithUpperCase(t *testing.T) {
	arn := "arn:aws:lambda:us-east-1:123456789012:function:My-Function"
	tagMap := BuildTagMap(arn, []string{"tag0:value0", "TAG1:VALUE1"})
	assert.Equal(t, 13, len(tagMap))
	assert.Equal(t, "lambda", tagMap["_dd.origin"])
	assert.Equal(t, "1", tagMap["_dd.compute_stats"])
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:my-function", tagMap["function_arn"])
	assert.Equal(t, "us-east-1", tagMap["region"])
	assert.Equal(t, "123456789012", tagMap["aws_account"])
	assert.Equal(t, "123456789012", tagMap["account_id"])
	assert.Equal(t, "my-function", tagMap["functionname"])
	assert.Equal(t, "my-function", tagMap["resource"])
	assert.Equal(t, "xxx", tagMap["dd_extension_version"])
	assert.Equal(t, "value0", tagMap["tag0"])
	assert.Equal(t, "value1", tagMap["tag1"])
	assert.True(t, tagMap["architecture"] == "x86_64" || tagMap["architecture"] == "arm64")
	assert.True(t, tagMap["runtime"] == "unknown" || tagMap["runtime"] == "provided.al2")
}

func TestBuildTagMapFromArnCompleteWithLatest(t *testing.T) {
	t.Setenv("AWS_LAMBDA_FUNCTION_VERSION", "$LATEST")
	arn := "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	tagMap := BuildTagMap(arn, []string{"tag0:value0", "TAG1:VALUE1"})
	assert.Equal(t, 13, len(tagMap))
	assert.Equal(t, "lambda", tagMap["_dd.origin"])
	assert.Equal(t, "1", tagMap["_dd.compute_stats"])
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:my-function", tagMap["function_arn"])
	assert.Equal(t, "us-east-1", tagMap["region"])
	assert.Equal(t, "123456789012", tagMap["aws_account"])
	assert.Equal(t, "123456789012", tagMap["account_id"])
	assert.Equal(t, "my-function", tagMap["functionname"])
	assert.Equal(t, "my-function", tagMap["resource"])
	assert.Equal(t, "xxx", tagMap["dd_extension_version"])
	assert.Equal(t, "value0", tagMap["tag0"])
	assert.Equal(t, "value1", tagMap["tag1"])
	assert.True(t, tagMap["architecture"] == "x86_64" || tagMap["architecture"] == "arm64")
	assert.True(t, tagMap["runtime"] == "unknown" || tagMap["runtime"] == "provided.al2")
}

func TestBuildTagMapFromArnCompleteWithVersionNumber(t *testing.T) {
	t.Setenv("AWS_LAMBDA_FUNCTION_VERSION", "888")
	arn := "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	tagMap := BuildTagMap(arn, []string{"tag0:value0", "TAG1:VALUE1"})
	assert.Equal(t, 14, len(tagMap))
	assert.Equal(t, "lambda", tagMap["_dd.origin"])
	assert.Equal(t, "1", tagMap["_dd.compute_stats"])
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:my-function", tagMap["function_arn"])
	assert.Equal(t, "us-east-1", tagMap["region"])
	assert.Equal(t, "123456789012", tagMap["aws_account"])
	assert.Equal(t, "123456789012", tagMap["account_id"])
	assert.Equal(t, "my-function", tagMap["functionname"])
	assert.Equal(t, "my-function:888", tagMap["resource"])
	assert.Equal(t, "888", tagMap["executedversion"])
	assert.Equal(t, "xxx", tagMap["dd_extension_version"])
	assert.Equal(t, "value0", tagMap["tag0"])
	assert.Equal(t, "value1", tagMap["tag1"])
	assert.True(t, tagMap["architecture"] == "x86_64" || tagMap["architecture"] == "arm64")
	assert.True(t, tagMap["runtime"] == "unknown" || tagMap["runtime"] == "provided.al2")
}

func TestAddTagInvalid(t *testing.T) {
	tagMap := map[string]string{
		"key_a": "value_a",
		"key_b": "value_b",
	}
	addTag(tagMap, "invalidTag")
	assert.Equal(t, 2, len(tagMap))
	assert.Equal(t, "value_a", tagMap["key_a"])
	assert.Equal(t, "value_b", tagMap["key_b"])
}

func TestAddTagInvalid2(t *testing.T) {
	tagMap := map[string]string{
		"key_a": "value_a",
		"key_b": "value_b",
	}
	addTag(tagMap, "invalidTag:invalid:invalid")
	assert.Equal(t, 2, len(tagMap))
	assert.Equal(t, "value_a", tagMap["key_a"])
	assert.Equal(t, "value_b", tagMap["key_b"])
}
func TestAddTagInvalid3(t *testing.T) {
	tagMap := map[string]string{
		"key_a": "value_a",
		"key_b": "value_b",
	}
	addTag(tagMap, "")
	assert.Equal(t, 2, len(tagMap))
	assert.Equal(t, "value_a", tagMap["key_a"])
	assert.Equal(t, "value_b", tagMap["key_b"])
}

func TestAddTag(t *testing.T) {
	tagMap := map[string]string{
		"key_a": "value_a",
		"key_b": "value_b",
	}
	addTag(tagMap, "VaLiD:TaG")
	assert.Equal(t, 3, len(tagMap))
	assert.Equal(t, "value_a", tagMap["key_a"])
	assert.Equal(t, "value_b", tagMap["key_b"])
	assert.Equal(t, "tag", tagMap["valid"])
}

func TestAddColdStartTagWithoutColdStart(t *testing.T) {
	generatedTags := AddColdStartTag([]string{
		"myTagName0:myTagValue0",
		"myTagName1:myTagValue1",
	}, false)

	assert.Equal(t, generatedTags, []string{
		"myTagName0:myTagValue0",
		"myTagName1:myTagValue1",
		"cold_start:false",
	})
}

func TestAddColdStartTagWithColdStart(t *testing.T) {
	generatedTags := AddColdStartTag([]string{
		"myTagName0:myTagValue0",
		"myTagName1:myTagValue1",
	}, true)

	assert.Equal(t, generatedTags, []string{
		"myTagName0:myTagValue0",
		"myTagName1:myTagValue1",
		"cold_start:true",
	})
}

func TestAddInitTypeTagWithoutInitType(t *testing.T) {
	generatedTags := AddInitTypeTag([]string{
		"myTagName0:myTagValue0",
		"myTagName1:myTagValue1",
	})
	assert.Equal(t, generatedTags, []string{
		"myTagName0:myTagValue0",
		"myTagName1:myTagValue1",
	})
}

func TestAddInitTypeTagWithInitType(t *testing.T) {
	t.Setenv(InitType, SnapStartValue)
	generatedTags := AddInitTypeTag([]string{
		"myTagName0:myTagValue0",
		"myTagName1:myTagValue1",
	})
	assert.Equal(t, generatedTags, []string{
		"myTagName0:myTagValue0",
		"myTagName1:myTagValue1",
		"init_type:snap-start",
	})
}

func TestBuildTagMapWithRuntimeAndMemoryTag(t *testing.T) {
	t.Setenv("AWS_LAMBDA_FUNCTION_VERSION", "888")
	t.Setenv("AWS_EXECUTION_ENV", "AWS_Lambda_java")
	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "128")
	arn := "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	tagMap := BuildTagMap(arn, []string{"tag0:value0", "TAG1:VALUE1"})
	assert.Equal(t, 15, len(tagMap))
	assert.Equal(t, "lambda", tagMap["_dd.origin"])
	assert.Equal(t, "1", tagMap["_dd.compute_stats"])
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:my-function", tagMap["function_arn"])
	assert.Equal(t, "us-east-1", tagMap["region"])
	assert.Equal(t, "123456789012", tagMap["aws_account"])
	assert.Equal(t, "123456789012", tagMap["account_id"])
	assert.Equal(t, "my-function", tagMap["functionname"])
	assert.Equal(t, "my-function:888", tagMap["resource"])
	assert.Equal(t, "xxx", tagMap["dd_extension_version"])
	assert.Equal(t, "value0", tagMap["tag0"])
	assert.Equal(t, "value1", tagMap["tag1"])
	assert.True(t, tagMap["runtime"] == "unknown" || tagMap["runtime"] == "provided.al2")
	assert.Equal(t, "128", tagMap["memorysize"])
	assert.True(t, tagMap["architecture"] == X86LambdaPlatform || tagMap["architecture"] == ArmLambdaPlatform)
}

func TestGetRuntimeFound(t *testing.T) {
	result := getRuntime("../proc/testData", "./testValidData", "AWS_EXECUTION_ENV")
	assert.Equal(t, "nodejs14.x", result)
}

func TestExtractRuntimeFromOsReleaseFileValid(t *testing.T) {
	result := getRuntimeFromOsReleaseFile("./testValid")
	assert.Equal(t, "provided.al2", result)
}

func TestExtractRuntimeFromOsReleaseFileInvalid(t *testing.T) {
	result := getRuntimeFromOsReleaseFile("./testInvalid")
	assert.Equal(t, "", result)
}

func TestExtractRuntimeFromOsReleaseFileInvalidPath(t *testing.T) {
	result := getRuntimeFromOsReleaseFile("/invalid/path")
	assert.Equal(t, "", result)
}

func TestCleanRuntimeValid(t *testing.T) {
	runtimes := []string{
		"AWS_Lambda_rapid",
		"AWS_Lambda_nodejs14.x",
		"AWS_Lambda_rapid",
	}
	assert.Equal(t, "AWS_Lambda_nodejs14.x", cleanRuntimes(runtimes))
}

func TestCleanRuntimeInvalid(t *testing.T) {
	runtimes := []string{
		"toto",
		"AWS_Lambda_nodejs14.x",
	}
	assert.Equal(t, "", cleanRuntimes(runtimes))
}
