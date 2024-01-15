// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

func TestInferredSpanCheck(t *testing.T) {
	inferredSpan := &pb.Span{
		Meta: map[string]string{"_inferred_span.tag_source": "self", "C": "D"},
	}
	lambdaSpanTagged := &pb.Span{
		Meta: map[string]string{"_inferred_span.tag_source": "lambda", "C": "D"},
	}
	lambdaSpanUntagged := &pb.Span{
		Meta: map[string]string{"foo": "bar", "C": "D"},
	}

	checkInferredSpan := CheckIsInferredSpan(inferredSpan)
	checkLambdaSpanTagged := CheckIsInferredSpan(lambdaSpanTagged)
	checkLambdaSpanUntagged := CheckIsInferredSpan(lambdaSpanUntagged)

	assert.True(t, checkInferredSpan)
	assert.False(t, checkLambdaSpanTagged)
	assert.False(t, checkLambdaSpanUntagged)
}

func TestFilterFunctionTags(t *testing.T) {
	tagsToFilter := map[string]string{
		"_inferred_span.tag_source": "self",
		"extra":                     "tag",
		"tag1":                      "value1",
		"functionname":              "test",
		"function_arn":              "test",
		"executedversion":           "test",
		"runtime":                   "test",
		"memorysize":                "test",
		"architecture":              "test",
		"env":                       "test",
		"version":                   "test",
		"service":                   "test",
		"region":                    "test",
		"account_id":                "test",
		"aws_account":               "test",
	}

	mockConfig := config.Mock(t)
	mockConfig.SetWithoutSource("tags", []string{"tag1:value1"})
	mockConfig.SetWithoutSource("extra_tags", []string{"extra:tag"})

	filteredTags := FilterFunctionTags(tagsToFilter)

	assert.Equal(t, filteredTags, map[string]string{
		"_inferred_span.tag_source": "self",
		"account_id":                "test",
		"aws_account":               "test",
		"region":                    "test",
	})

	// ensure DD_TAGS are filtered out
	assert.NotEqual(t, filteredTags["tag1"], "value1")

	// ensure DD_EXTRA_TAGS are filtered out
	assert.NotEqual(t, filteredTags["extra"], "tag")

	// ensure all function specific tags are filtered out
	assert.NotEqual(t, filteredTags["functionname"], "test")
	assert.NotEqual(t, filteredTags["function_arn"], "test")
	assert.NotEqual(t, filteredTags["executedversion"], "test")
	assert.NotEqual(t, filteredTags["runtime"], "test")
	assert.NotEqual(t, filteredTags["memorysize"], "test")
	assert.NotEqual(t, filteredTags["env"], "test")
	assert.NotEqual(t, filteredTags["version"], "test")
	assert.NotEqual(t, filteredTags["service"], "test")

	// ensure generic aws tags are not filtered out
	assert.Equal(t, filteredTags["region"], "test")
	assert.Equal(t, filteredTags["account_id"], "test")
	assert.Equal(t, filteredTags["aws_account"], "test")
}

func TestIsInferredSpansEnabledWhileTrue(t *testing.T) {
	setEnvVars(t, "true", "True")
	isEnabled := IsInferredSpansEnabled()
	assert.True(t, isEnabled)
}
func TestIsInferredSpansEnabledWhileFalse(t *testing.T) {
	setEnvVars(t, "true", "false")
	isEnabled := IsInferredSpansEnabled()
	assert.False(t, isEnabled)
}

func TestIsInferredSpansEnabledWhileInvalid(t *testing.T) {
	setEnvVars(t, "true", "42")
	isEnabled := IsInferredSpansEnabled()
	assert.False(t, isEnabled)

}

func setEnvVars(t *testing.T, trace string, managedServices string) {
	t.Setenv("DD_TRACE_ENABLED", trace)
	t.Setenv("DD_TRACE_MANAGED_SERVICES", managedServices)
}
