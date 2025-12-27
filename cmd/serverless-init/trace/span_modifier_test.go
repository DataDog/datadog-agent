// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"testing"

	idx "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/stretchr/testify/assert"
)

func TestCloudRunJobsSpanModifier(t *testing.T) {
	jobTraceID := uint64(12345)
	jobSpanID := uint64(67890)

	modifier := NewCloudRunJobsSpanModifier(jobTraceID, jobSpanID)

	// Create a user span
	st := idx.NewStringTable()
	userSpan := idx.NewInternalSpan(st, &idx.Span{
		ServiceRef:  st.Add("user-service"),
		NameRef:     st.Add("user.operation"),
		ResourceRef: st.Add("user-resource"),
		SpanID:      123,
		Attributes:  map[uint32]*idx.AnyValue{},
	})
	userChunk := idx.NewInternalTraceChunk(st, 1, "user-origin", map[uint32]*idx.AnyValue{}, []*idx.InternalSpan{userSpan}, false, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, 4)
	originalTraceID := userChunk.LegacyTraceID()
	originalSpanID := userSpan.SpanID()

	// Modify the span
	modifier.ModifySpan(userChunk, userSpan)

	// Verify the span was reparented
	assert.Equal(t, jobTraceID, userChunk.LegacyTraceID())
	assert.Equal(t, jobSpanID, userSpan.ParentID())
	assert.Equal(t, originalSpanID, userSpan.SpanID())             // SpanID should not change
	assert.NotEqual(t, originalTraceID, userChunk.LegacyTraceID()) // TraceID should change
}

func TestCloudRunJobsSpanModifierDoesNotModifyJobSpan(t *testing.T) {
	jobTraceID := uint64(12345)
	jobSpanID := uint64(67890)

	modifier := NewCloudRunJobsSpanModifier(jobTraceID, jobSpanID)

	// Create the job span itself
	st := idx.NewStringTable()
	jobSpan := idx.NewInternalSpan(st, &idx.Span{
		ServiceRef:  st.Add("gcp.run.job"),
		NameRef:     st.Add("gcp.run.job.task"),
		ResourceRef: st.Add("my-job"),
		SpanID:      123,
		Attributes:  map[uint32]*idx.AnyValue{},
	})
	userChunk := idx.NewInternalTraceChunk(st, 1, "user-origin", map[uint32]*idx.AnyValue{}, []*idx.InternalSpan{jobSpan}, false, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, 4)
	originalTraceID := userChunk.LegacyTraceID()
	originalParentID := jobSpan.ParentID()

	// Modify should not affect the job span
	modifier.ModifySpan(userChunk, jobSpan)

	// Verify the job span was not modified
	assert.Equal(t, originalTraceID, userChunk.LegacyTraceID())
	assert.Equal(t, originalParentID, jobSpan.ParentID())
}
