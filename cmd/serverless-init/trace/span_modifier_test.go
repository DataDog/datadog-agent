// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCloudRunJobsSpanModifier(t *testing.T) {
	jobTraceID := uint64(12345)
	jobSpanID := uint64(67890)

	modifier := NewCloudRunJobsSpanModifier(jobTraceID, jobSpanID)

	// Create a user span
	userSpan := InitSpan("user-service", "user.operation", "user-resource", "web", time.Now().UnixNano(), map[string]string{})
	originalTraceID := userSpan.TraceID
	originalSpanID := userSpan.SpanID

	// Modify the span
	modifier.ModifySpan(nil, userSpan)

	// Verify the span was reparented
	assert.Equal(t, jobTraceID, userSpan.TraceID)
	assert.Equal(t, jobSpanID, userSpan.ParentID)
	assert.Equal(t, originalSpanID, userSpan.SpanID)      // SpanID should not change
	assert.NotEqual(t, originalTraceID, userSpan.TraceID) // TraceID should change
}

func TestCloudRunJobsSpanModifierDoesNotModifyJobSpan(t *testing.T) {
	jobTraceID := uint64(12345)
	jobSpanID := uint64(67890)

	modifier := NewCloudRunJobsSpanModifier(jobTraceID, jobSpanID)

	// Create the job span itself
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	originalTraceID := jobSpan.TraceID
	originalParentID := jobSpan.ParentID

	// Modify should not affect the job span
	modifier.ModifySpan(nil, jobSpan)

	// Verify the job span was not modified
	assert.Equal(t, originalTraceID, jobSpan.TraceID)
	assert.Equal(t, originalParentID, jobSpan.ParentID)
}
