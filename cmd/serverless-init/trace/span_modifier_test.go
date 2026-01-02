// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"sync"
	"testing"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/stretchr/testify/assert"
)

func TestCloudRunJobsSpanModifier_PreservesHierarchy(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create a hierarchy: root → child → grandchild
	rootSpan := InitSpan("user-service", "root.operation", "root-resource", "web", time.Now().UnixNano(), map[string]string{})
	rootSpan.ParentID = 0 // Explicit root span

	childSpan := InitSpan("user-service", "child.operation", "child-resource", "web", time.Now().UnixNano(), map[string]string{})
	childSpan.ParentID = rootSpan.SpanID

	grandchildSpan := InitSpan("user-service", "grandchild.operation", "grandchild-resource", "web", time.Now().UnixNano(), map[string]string{})
	grandchildSpan.ParentID = childSpan.SpanID

	// Modify spans (order matters: root first, then children)
	modifier.ModifySpan(nil, rootSpan)
	modifier.ModifySpan(nil, childSpan)
	modifier.ModifySpan(nil, grandchildSpan)

	// Verify: only root span reparented, children preserve their hierarchy
	assert.Equal(t, jobSpan.SpanID, rootSpan.ParentID, "Root span should be reparented under job span")
	assert.Equal(t, rootSpan.SpanID, childSpan.ParentID, "Child span should still point to root span")
	assert.Equal(t, childSpan.SpanID, grandchildSpan.ParentID, "Grandchild span should still point to child span")
}

func TestCloudRunJobsSpanModifier_AdoptsFirstTraceID(t *testing.T) {
	// Create job span with its own TraceID
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	originalJobTraceID := jobSpan.TraceID

	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create user root span with different TraceID
	userSpan := InitSpan("user-service", "user.operation", "user-resource", "web", time.Now().UnixNano(), map[string]string{})
	userSpan.ParentID = 0
	userTraceID := userSpan.TraceID

	// Modify the user span
	modifier.ModifySpan(nil, userSpan)

	// Verify: job span adopts user's TraceID
	assert.Equal(t, userTraceID, jobSpan.TraceID, "Job span should adopt user's TraceID")
	assert.NotEqual(t, originalJobTraceID, jobSpan.TraceID, "Job span TraceID should change")
	assert.Equal(t, jobSpan.SpanID, userSpan.ParentID, "User span should be reparented under job span")
}

func TestCloudRunJobsSpanModifier_MultipleTraces(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create first trace's root span
	firstSpan := InitSpan("service-a", "operation-a", "resource-a", "web", time.Now().UnixNano(), map[string]string{})
	firstSpan.ParentID = 0
	firstTraceID := firstSpan.TraceID

	// Create second trace's root span (different TraceID)
	secondSpan := InitSpan("service-b", "operation-b", "resource-b", "web", time.Now().UnixNano(), map[string]string{})
	secondSpan.ParentID = 0
	secondTraceID := secondSpan.TraceID

	// Ensure they have different TraceIDs
	assert.NotEqual(t, firstTraceID, secondTraceID, "Test setup: traces should have different IDs")

	// Modify both spans
	modifier.ModifySpan(nil, firstSpan)
	modifier.ModifySpan(nil, secondSpan)

	// Verify: first trace adopted, second trace ignored
	assert.Equal(t, firstTraceID, jobSpan.TraceID, "Job span should adopt first trace's ID")
	assert.Equal(t, jobSpan.SpanID, firstSpan.ParentID, "First span should be reparented")
	assert.Equal(t, uint64(0), secondSpan.ParentID, "Second span should NOT be reparented (different trace)")
}

func TestCloudRunJobsSpanModifier_NoUserTraces(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	originalTraceID := jobSpan.TraceID
	_ = NewCloudRunJobsSpanModifier(jobSpan)

	// Don't send any user spans - just verify job span unchanged
	assert.Equal(t, originalTraceID, jobSpan.TraceID, "Job span should keep its own TraceID when no user traces seen")
}

func TestCloudRunJobsSpanModifier_ConcurrentAccess(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create multiple root spans
	const numSpans = 100
	spans := make([]*pb.Span, numSpans)
	for i := 0; i < numSpans; i++ {
		spans[i] = InitSpan("user-service", "operation", "resource", "web", time.Now().UnixNano(), map[string]string{})
		spans[i].ParentID = 0
	}

	// Modify spans concurrently
	var wg sync.WaitGroup
	for i := 0; i < numSpans; i++ {
		wg.Add(1)
		go func(span *pb.Span) {
			defer wg.Done()
			modifier.ModifySpan(nil, span)
		}(spans[i])
	}
	wg.Wait()

	// Verify: only one TraceID was adopted, and all matching spans were reparented
	adoptedTraceID := jobSpan.TraceID
	assert.NotEqual(t, uint64(0), adoptedTraceID, "Job span should have adopted a TraceID")

	reparentedCount := 0
	for _, span := range spans {
		if span.TraceID == adoptedTraceID {
			assert.Equal(t, jobSpan.SpanID, span.ParentID, "Span with adopted TraceID should be reparented")
			reparentedCount++
		} else {
			assert.Equal(t, uint64(0), span.ParentID, "Span with different TraceID should not be reparented")
		}
	}
	assert.Greater(t, reparentedCount, 0, "At least one span should have been reparented")
}

func TestCloudRunJobsSpanModifier_ChildBeforeRoot(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create child and root spans
	rootSpan := InitSpan("user-service", "root.operation", "root-resource", "web", time.Now().UnixNano(), map[string]string{})
	rootSpan.ParentID = 0

	childSpan := InitSpan("user-service", "child.operation", "child-resource", "web", time.Now().UnixNano(), map[string]string{})
	childSpan.ParentID = rootSpan.SpanID

	// Modify in REVERSE order: child arrives before root
	modifier.ModifySpan(nil, childSpan)
	modifier.ModifySpan(nil, rootSpan)

	// Verify: child not modified (has ParentID != 0), root reparented
	assert.Equal(t, rootSpan.SpanID, childSpan.ParentID, "Child span should still point to root span")
	assert.Equal(t, jobSpan.SpanID, rootSpan.ParentID, "Root span should be reparented under job span")
}

func TestCloudRunJobsSpanModifier_JobSpanNeverModified(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	originalTraceID := jobSpan.TraceID
	originalSpanID := jobSpan.SpanID
	originalParentID := jobSpan.ParentID

	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Try to modify the job span itself
	modifier.ModifySpan(nil, jobSpan)

	// Verify: job span's ParentID and SpanID unchanged
	assert.Equal(t, originalSpanID, jobSpan.SpanID, "Job span's SpanID should not change")
	assert.Equal(t, originalParentID, jobSpan.ParentID, "Job span's ParentID should not change")
	// Note: TraceID can change when adopting, but only when processing USER spans, not the job span itself
	assert.Equal(t, originalTraceID, jobSpan.TraceID, "Job span's TraceID should not change when processing itself")
}

func TestCloudRunJobsSpanModifier_TraceIDAdoptionOnlyFromRootSpans(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	originalJobTraceID := jobSpan.TraceID

	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create a child span (ParentID != 0) that arrives FIRST
	childSpan := InitSpan("user-service", "child.operation", "child-resource", "web", time.Now().UnixNano(), map[string]string{})
	childSpan.ParentID = 12345 // Non-zero parent
	childTraceID := childSpan.TraceID

	// Modify child span first
	modifier.ModifySpan(nil, childSpan)

	// Verify: job span should NOT adopt child's TraceID (child is not a root)
	assert.Equal(t, originalJobTraceID, jobSpan.TraceID, "Job span should not adopt TraceID from child spans")

	// Now create a root span
	rootSpan := InitSpan("user-service", "root.operation", "root-resource", "web", time.Now().UnixNano(), map[string]string{})
	rootSpan.ParentID = 0
	rootTraceID := rootSpan.TraceID

	// Modify root span
	modifier.ModifySpan(nil, rootSpan)

	// Verify: job span NOW adopts root's TraceID
	assert.Equal(t, rootTraceID, jobSpan.TraceID, "Job span should adopt TraceID from root spans")
	assert.NotEqual(t, childTraceID, jobSpan.TraceID, "Job span should not have adopted child's TraceID")
}

func TestCloudRunJobsSpanModifier_128BitTraceID(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create a root span with a 128-bit trace ID (simulating a tracer that uses 128-bit IDs)
	rootSpan := InitSpan("user-service", "root.operation", "root-resource", "web", time.Now().UnixNano(), map[string]string{})
	rootSpan.ParentID = 0
	rootSpan.Meta = map[string]string{
		"_dd.p.tid": "6958127700000000", // High 64 bits of trace ID (hex-encoded)
	}

	// Modify the root span
	modifier.ModifySpan(nil, rootSpan)

	// Verify: job span adopted both low and high 64 bits
	assert.Equal(t, rootSpan.TraceID, jobSpan.TraceID, "Job span should adopt low 64 bits (TraceID)")
	assert.Equal(t, "6958127700000000", jobSpan.Meta["_dd.p.tid"], "Job span should adopt high 64 bits (_dd.p.tid)")
	assert.Equal(t, jobSpan.SpanID, rootSpan.ParentID, "Root span should be reparented under job span")
}

func TestCloudRunJobsSpanModifier_128BitTraceID_NoHighBits(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create a root span WITHOUT high bits (64-bit trace ID only)
	rootSpan := InitSpan("user-service", "root.operation", "root-resource", "web", time.Now().UnixNano(), map[string]string{})
	rootSpan.ParentID = 0
	// No _dd.p.tid tag

	// Modify the root span
	modifier.ModifySpan(nil, rootSpan)

	// Verify: job span adopted low 64 bits, no high bits set
	assert.Equal(t, rootSpan.TraceID, jobSpan.TraceID, "Job span should adopt low 64 bits (TraceID)")
	assert.NotContains(t, jobSpan.Meta, "_dd.p.tid", "Job span should not have _dd.p.tid when user span doesn't have it")
	assert.Equal(t, jobSpan.SpanID, rootSpan.ParentID, "Root span should be reparented under job span")
}
