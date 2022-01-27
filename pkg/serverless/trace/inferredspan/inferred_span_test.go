// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
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

	checkInferredSpan := Check(inferredSpan)
	checkLambdaSpanTagged := Check(lambdaSpanTagged)
	checkLambdaSpanUntagged := Check(lambdaSpanUntagged)

	assert.True(t, checkInferredSpan)
	assert.False(t, checkLambdaSpanTagged)
	assert.False(t, checkLambdaSpanUntagged)
}

func TestFilterFunctionTags(t *testing.T) {
	tagsToFilter := map[string]string{"_inferred_span.tag_source": "self", "functionname": "lambda", "extra": "tag", "tag1": "value1"}

	mockConfig := config.Mock()
	mockConfig.Set("tags", []string{"tag1:value1"})
	mockConfig.Set("extra_tags", []string{"extra:tag"})

	assert.Equal(t, tagsToFilter["functionname"], "lambda")
	assert.Equal(t, tagsToFilter["extra"], "tag")
	assert.Equal(t, tagsToFilter["tag1"], "value1")

	FilterFunctionTags(&tagsToFilter)

	assert.Equal(t, tagsToFilter, map[string]string{"_inferred_span.tag_source": "self"})
	assert.NotEqual(t, tagsToFilter["functionname"], "lambda")
	assert.NotEqual(t, tagsToFilter["extra"], "tag")
	assert.NotEqual(t, tagsToFilter["tag1"], "value1")
}
