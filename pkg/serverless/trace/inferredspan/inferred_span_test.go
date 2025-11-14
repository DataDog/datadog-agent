// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

func TestInferredSpanCheck(t *testing.T) {
	// Create an inferred span with tag_source = "self"
	stringTable1 := idx.NewStringTable()
	inferredSpan := idx.NewInternalSpan(stringTable1, &idx.Span{})
	inferredSpan.SetAttributeFromString("_inferred_span.tag_source", "self")
	inferredSpan.SetAttributeFromString("C", "D")

	// Create a lambda span with tag_source = "lambda"
	stringTable2 := idx.NewStringTable()
	lambdaSpanTagged := idx.NewInternalSpan(stringTable2, &idx.Span{})
	lambdaSpanTagged.SetAttributeFromString("_inferred_span.tag_source", "lambda")
	lambdaSpanTagged.SetAttributeFromString("C", "D")

	// Create an untagged lambda span (no tag_source)
	stringTable3 := idx.NewStringTable()
	lambdaSpanUntagged := idx.NewInternalSpan(stringTable3, &idx.Span{})
	lambdaSpanUntagged.SetAttributeFromString("foo", "bar")
	lambdaSpanUntagged.SetAttributeFromString("C", "D")

	checkInferredSpan := CheckIsInferredSpan(inferredSpan)
	checkLambdaSpanTagged := CheckIsInferredSpan(lambdaSpanTagged)
	checkLambdaSpanUntagged := CheckIsInferredSpan(lambdaSpanUntagged)

	assert.True(t, checkInferredSpan)
	assert.False(t, checkLambdaSpanTagged)
	assert.False(t, checkLambdaSpanUntagged)
}

func TestFilterFunctionTags(t *testing.T) {
	// Create a span with tags that should be filtered
	stringTable := idx.NewStringTable()
	span := idx.NewInternalSpan(stringTable, &idx.Span{})

	// Set all the tags as attributes on the span
	tagsToSet := map[string]string{
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

	for key, value := range tagsToSet {
		span.SetAttributeFromString(key, value)
	}

	// Configure DD_TAGS and DD_EXTRA_TAGS via environment variables
	t.Setenv("DD_TAGS", "tag1:value1")
	t.Setenv("DD_EXTRA_TAGS", "extra:tag")
	_ = configmock.New(t)

	// Call FilterFunctionTags
	FilterFunctionTags(span)

	// Verify that the tag source is still present
	tagSource, ok := span.GetAttributeAsString("_inferred_span.tag_source")
	assert.True(t, ok)
	assert.Equal(t, "self", tagSource)

	// Verify that generic AWS tags are NOT filtered out
	region, ok := span.GetAttributeAsString("region")
	assert.True(t, ok)
	assert.Equal(t, "test", region)

	accountID, ok := span.GetAttributeAsString("account_id")
	assert.True(t, ok)
	assert.Equal(t, "test", accountID)

	awsAccount, ok := span.GetAttributeAsString("aws_account")
	assert.True(t, ok)
	assert.Equal(t, "test", awsAccount)

	// Verify that DD_TAGS are filtered out
	_, ok = span.GetAttributeAsString("tag1")
	assert.False(t, ok)

	// Verify that DD_EXTRA_TAGS are filtered out
	_, ok = span.GetAttributeAsString("extra")
	assert.False(t, ok)

	// Verify that all function specific tags are filtered out
	_, ok = span.GetAttributeAsString("functionname")
	assert.False(t, ok)

	_, ok = span.GetAttributeAsString("function_arn")
	assert.False(t, ok)

	_, ok = span.GetAttributeAsString("executedversion")
	assert.False(t, ok)

	_, ok = span.GetAttributeAsString("runtime")
	assert.False(t, ok)

	_, ok = span.GetAttributeAsString("memorysize")
	assert.False(t, ok)

	// Note: "env" and "version" are special fields on InternalSpan and cannot be deleted via DeleteAttribute
	// They are stored as dedicated fields, not as regular attributes
	// So we expect them to still be present even after FilterFunctionTags is called
	assert.Equal(t, "", span.Service())
	assert.Equal(t, "", span.Env())
	assert.Equal(t, "", span.Version())

	_, ok = span.GetAttributeAsString("architecture")
	assert.False(t, ok)
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
	configmock.SetDefaultConfigType(t, "yaml")
	t.Setenv("DD_TRACE_ENABLED", trace)
	t.Setenv("DD_TRACE_MANAGED_SERVICES", managedServices)
}
