// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"sync"
	"testing"
	"time"

	idx "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/stretchr/testify/assert"
)

// Helper function to create an InternalSpan with its chunk for testing
func createTestSpan(service, name, resource string, parentID uint64) (*idx.InternalTraceChunk, *idx.InternalSpan) {
	st := idx.NewStringTable()
	span := idx.NewInternalSpan(st, &idx.Span{
		ServiceRef:  st.Add(service),
		NameRef:     st.Add(name),
		ResourceRef: st.Add(resource),
		SpanID:      rand.Uint64(),
		ParentID:    parentID,
		Attributes:  map[uint32]*idx.AnyValue{},
	})

	// Create a random 128-bit trace ID
	traceID := make([]byte, 16)
	binary.BigEndian.PutUint64(traceID[:8], rand.Uint64())
	binary.BigEndian.PutUint64(traceID[8:], rand.Uint64())

	chunk := idx.NewInternalTraceChunk(st, 1, "test-origin", map[uint32]*idx.AnyValue{}, []*idx.InternalSpan{span}, false, traceID, 0)
	return chunk, span
}

// Helper to create a span with a specific trace ID
func createTestSpanWithTraceID(service, name, resource string, parentID uint64, traceID []byte) (*idx.InternalTraceChunk, *idx.InternalSpan) {
	st := idx.NewStringTable()
	span := idx.NewInternalSpan(st, &idx.Span{
		ServiceRef:  st.Add(service),
		NameRef:     st.Add(name),
		ResourceRef: st.Add(resource),
		SpanID:      rand.Uint64(),
		ParentID:    parentID,
		Attributes:  map[uint32]*idx.AnyValue{},
	})

	chunk := idx.NewInternalTraceChunk(st, 1, "test-origin", map[uint32]*idx.AnyValue{}, []*idx.InternalSpan{span}, false, traceID, 0)
	return chunk, span
}

func TestCloudRunJobsSpanModifier_PreservesHierarchy(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create a shared trace ID for the hierarchy
	traceID := make([]byte, 16)
	binary.BigEndian.PutUint64(traceID[8:], rand.Uint64())

	// Create a hierarchy: root → child → grandchild
	rootChunk, rootSpan := createTestSpanWithTraceID("user-service", "root.operation", "root-resource", 0, traceID)

	_, childSpan := createTestSpanWithTraceID("user-service", "child.operation", "child-resource", rootSpan.SpanID(), traceID)
	_, grandchildSpan := createTestSpanWithTraceID("user-service", "grandchild.operation", "grandchild-resource", childSpan.SpanID(), traceID)

	// Modify spans (order matters: root first, then children)
	modifier.ModifySpan(rootChunk, rootSpan)
	modifier.ModifySpan(rootChunk, childSpan)
	modifier.ModifySpan(rootChunk, grandchildSpan)

	// Verify: only root span reparented, children preserve their hierarchy
	assert.Equal(t, jobSpan.SpanID, rootSpan.ParentID(), "Root span should be reparented under job span")
	assert.Equal(t, rootSpan.SpanID(), childSpan.ParentID(), "Child span should still point to root span")
	assert.Equal(t, childSpan.SpanID(), grandchildSpan.ParentID(), "Grandchild span should still point to child span")
}

func TestCloudRunJobsSpanModifier_AdoptsFirstTraceID(t *testing.T) {
	// Create job span with its own TraceID
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	originalJobTraceID := jobSpan.TraceID

	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create user root span with different TraceID
	userChunk, userSpan := createTestSpan("user-service", "user.operation", "user-resource", 0)

	// Modify the user span
	modifier.ModifySpan(userChunk, userSpan)

	// Verify: job span adopts user's TraceID
	assert.Equal(t, userChunk.LegacyTraceID(), jobSpan.TraceID, "Job span should adopt user's TraceID")
	assert.NotEqual(t, originalJobTraceID, jobSpan.TraceID, "Job span TraceID should change")
	assert.Equal(t, jobSpan.SpanID, userSpan.ParentID(), "User span should be reparented under job span")
}

func TestCloudRunJobsSpanModifier_MultipleTraces(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create first trace's root span
	firstChunk, firstSpan := createTestSpan("service-a", "operation-a", "resource-a", 0)

	// Create second trace's root span (different TraceID)
	secondChunk, secondSpan := createTestSpan("service-b", "operation-b", "resource-b", 0)

	// Ensure they have different TraceIDs
	assert.False(t, bytes.Equal(firstChunk.TraceID, secondChunk.TraceID), "Test setup: first and second spans should have different TraceIDs")

	// Modify both spans
	modifier.ModifySpan(firstChunk, firstSpan)
	modifier.ModifySpan(secondChunk, secondSpan)

	// Verify: first trace adopted, second trace ignored
	assert.Equal(t, firstChunk.LegacyTraceID(), jobSpan.TraceID, "Job span should adopt first trace's ID")
	assert.Equal(t, jobSpan.SpanID, firstSpan.ParentID(), "First span should be reparented")
	assert.Equal(t, uint64(0), secondSpan.ParentID(), "Second span should NOT be reparented (different trace)")
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

	// Create multiple root spans with different trace IDs
	const numSpans = 100
	chunks := make([]*idx.InternalTraceChunk, numSpans)
	spans := make([]*idx.InternalSpan, numSpans)
	for i := 0; i < numSpans; i++ {
		chunks[i], spans[i] = createTestSpan("user-service", "operation", "resource", 0)
	}

	// Modify spans concurrently
	var wg sync.WaitGroup
	for i := 0; i < numSpans; i++ {
		wg.Add(1)
		go func(chunk *idx.InternalTraceChunk, span *idx.InternalSpan) {
			defer wg.Done()
			modifier.ModifySpan(chunk, span)
		}(chunks[i], spans[i])
	}
	wg.Wait()

	// Verify: only one TraceID was adopted, and all matching spans were reparented
	reparentedCount := 0
	for i, span := range spans {
		if chunks[i].LegacyTraceID() == jobSpan.TraceID {
			assert.Equal(t, jobSpan.SpanID, span.ParentID(), "Span with adopted TraceID should be reparented")
			reparentedCount++
		} else {
			assert.Equal(t, uint64(0), span.ParentID(), "Span with different TraceID should not be reparented")
		}
	}
	assert.Greater(t, reparentedCount, 0, "At least one span should have been reparented")
}

func TestCloudRunJobsSpanModifier_ChildBeforeRoot(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create a shared trace ID
	traceID := make([]byte, 16)
	binary.BigEndian.PutUint64(traceID[8:], rand.Uint64())

	// Create root span first (to get its SpanID)
	rootChunk, rootSpan := createTestSpanWithTraceID("user-service", "root.operation", "root-resource", 0, traceID)

	// Create child span with SAME TraceID as root (realistic scenario)
	childChunk, childSpan := createTestSpanWithTraceID("user-service", "child.operation", "child-resource", rootSpan.SpanID(), traceID)

	// Modify in REVERSE order: child arrives before root
	// With new behavior: child's TraceID is adopted, then root (with same TraceID) is reparented
	modifier.ModifySpan(childChunk, childSpan)
	modifier.ModifySpan(rootChunk, rootSpan)

	// Verify: child not modified (has ParentID != 0), root reparented
	assert.Equal(t, rootSpan.SpanID(), childSpan.ParentID(), "Child span should still point to root span")
	assert.Equal(t, jobSpan.SpanID, rootSpan.ParentID(), "Root span should be reparented under job span")
	assert.Equal(t, rootChunk.LegacyTraceID(), jobSpan.TraceID, "Job span should have adopted the trace ID")
}

func TestCloudRunJobsSpanModifier_TracePropagation(t *testing.T) {
	// Test trace propagation scenario: all spans have non-zero ParentIDs
	// This happens when trace context is propagated into the Cloud Run Job
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	originalJobTraceID := jobSpan.TraceID
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create a shared trace ID for propagated trace
	traceID := make([]byte, 16)
	binary.BigEndian.PutUint64(traceID[8:], 0xABCD1234ABCD1234)

	// Simulate propagated trace - all spans have ParentID != 0
	// (they're children of spans from the calling service)
	externalParentID := uint64(0x9999888877776666)

	span1Chunk, span1 := createTestSpanWithTraceID("user-service", "operation-1", "resource-1", externalParentID, traceID)
	span2Chunk, span2 := createTestSpanWithTraceID("user-service", "operation-2", "resource-2", span1.SpanID(), traceID)

	// Modify spans
	modifier.ModifySpan(span1Chunk, span1)
	modifier.ModifySpan(span2Chunk, span2)

	// Verify: TraceID is adopted from first span
	assert.Equal(t, span1Chunk.LegacyTraceID(), jobSpan.TraceID, "Job span should adopt propagated TraceID")
	assert.NotEqual(t, originalJobTraceID, jobSpan.TraceID, "Job span TraceID should have changed")

	// Verify: No spans are reparented (none have ParentID == 0)
	assert.Equal(t, externalParentID, span1.ParentID(), "Span1 should keep its original parent")
	assert.Equal(t, span1.SpanID(), span2.ParentID(), "Span2 should keep its original parent")

	// Note: In this scenario, the job span is "floating" (unparented) within the trace.
	// This is the expected trade-off for supporting trace propagation.
}

func TestCloudRunJobsSpanModifier_JobSpanNeverModified(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	originalTraceID := jobSpan.TraceID
	originalSpanID := jobSpan.SpanID
	originalParentID := jobSpan.ParentID

	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create an idx span representing the job span itself
	st := idx.NewStringTable()
	jobIdxSpan := idx.NewInternalSpan(st, &idx.Span{
		ServiceRef:  st.Add("gcp.run.job"),
		NameRef:     st.Add("gcp.run.job.task"),
		ResourceRef: st.Add("my-job"),
		SpanID:      jobSpan.SpanID,
		ParentID:    jobSpan.ParentID,
		Attributes:  map[uint32]*idx.AnyValue{},
	})

	traceID := make([]byte, 16)
	binary.BigEndian.PutUint64(traceID[8:], jobSpan.TraceID)
	jobChunk := idx.NewInternalTraceChunk(st, 1, "test-origin", map[uint32]*idx.AnyValue{}, []*idx.InternalSpan{jobIdxSpan}, false, traceID, 0)

	// Try to modify the job span itself
	modifier.ModifySpan(jobChunk, jobIdxSpan)

	// Verify: job span's ParentID and SpanID unchanged
	assert.Equal(t, originalSpanID, jobSpan.SpanID, "Job span's SpanID should not change")
	assert.Equal(t, originalParentID, jobSpan.ParentID, "Job span's ParentID should not change")
	// Note: TraceID can change when adopting, but only when processing USER spans, not the job span itself
	assert.Equal(t, originalTraceID, jobSpan.TraceID, "Job span's TraceID should not change when processing itself")
}

func TestCloudRunJobsSpanModifier_128BitTraceID(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create a root span with a 128-bit trace ID
	traceID := make([]byte, 16)
	binary.BigEndian.PutUint64(traceID[:8], 0x6958127700000000) // High 64 bits
	binary.BigEndian.PutUint64(traceID[8:], rand.Uint64())      // Low 64 bits

	st := idx.NewStringTable()
	rootSpan := idx.NewInternalSpan(st, &idx.Span{
		ServiceRef:  st.Add("user-service"),
		NameRef:     st.Add("root.operation"),
		ResourceRef: st.Add("root-resource"),
		SpanID:      rand.Uint64(),
		ParentID:    0,
		Attributes:  map[uint32]*idx.AnyValue{},
	})

	// Add the _dd.p.tid attribute for high bits
	rootChunk := idx.NewInternalTraceChunk(st, 1, "test-origin", map[uint32]*idx.AnyValue{}, []*idx.InternalSpan{rootSpan}, false, traceID, 0)
	rootChunk.SetStringAttribute("_dd.p.tid", "6958127700000000")

	// Modify the root span
	modifier.ModifySpan(rootChunk, rootSpan)

	// Verify: job span adopted the trace ID
	assert.Equal(t, rootChunk.LegacyTraceID(), jobSpan.TraceID, "Job span should adopt TraceID low bits")
	assert.Equal(t, "6958127700000000", jobSpan.Meta["_dd.p.tid"], "Job span should adopt TraceID high bits")
	assert.Equal(t, jobSpan.SpanID, rootSpan.ParentID(), "Root span should be reparented under job span")
}

func TestCloudRunJobsSpanModifier_128BitTraceID_NoHighBits(t *testing.T) {
	// Create job span
	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// Create a root span WITHOUT high bits (64-bit trace ID only - high 8 bytes are zero)
	traceID := make([]byte, 16)
	binary.BigEndian.PutUint64(traceID[:8], 0)             // High 64 bits = 0
	binary.BigEndian.PutUint64(traceID[8:], rand.Uint64()) // Low 64 bits

	rootChunk, rootSpan := createTestSpanWithTraceID("user-service", "root.operation", "root-resource", 0, traceID)

	// Modify the root span
	modifier.ModifySpan(rootChunk, rootSpan)

	// Verify: job span adopted the 64-bit trace ID
	assert.Equal(t, rootChunk.LegacyTraceID(), jobSpan.TraceID, "Job span should adopt TraceID")
	assert.Equal(t, jobSpan.SpanID, rootSpan.ParentID(), "Root span should be reparented under job span")
}

func TestCloudRunJobsSpanModifier_MixedTraceIDBitWidths_128BitFirst(t *testing.T) {
	// Test backward compatibility: spans from the same trace may have different
	// bit widths (some with _dd.p.tid, some without). Per Datadog documentation,
	// these should be treated as matching if the low 64 bits match.
	// https://docs.datadoghq.com/tracing/guide/span_and_trace_id_format/

	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// First span arrives with 128-bit trace ID (has high bits)
	rootSpan := InitSpan("user-service", "root.operation", "root-resource", "web", time.Now().UnixNano(), map[string]string{})
	rootSpan.ParentID = 0
	traceutil.SetTraceIDHigh(rootSpan, "6958127700000000")

	// Second span from same trace arrives with only 64-bit trace ID (no high bits)
	// This can happen when mixing tracers with different 128-bit support
	childSpan := InitSpan("user-service", "child.operation", "child-resource", "web", time.Now().UnixNano(), map[string]string{})
	childSpan.TraceID = rootSpan.TraceID // Same low 64 bits
	childSpan.ParentID = rootSpan.SpanID
	// No high bits - simulates a tracer that doesn't support 128-bit IDs

	// Modify both spans
	modifier.ModifySpan(nil, rootSpan)
	modifier.ModifySpan(nil, childSpan)

	// Verify: job span adopted the full 128-bit trace ID from root
	assert.True(t, traceutil.SameTraceID(jobSpan, rootSpan), "Job span should match root span")

	// Verify: child span (64-bit only) is still recognized as same trace
	// because SameTraceID only compares low 64 bits when one span lacks high bits
	assert.True(t, traceutil.SameTraceID(jobSpan, childSpan), "Job span should match child span despite missing high bits")

	// Verify hierarchy preserved
	assert.Equal(t, jobSpan.SpanID, rootSpan.ParentID, "Root span should be reparented under job span")
	assert.Equal(t, rootSpan.SpanID, childSpan.ParentID, "Child span should keep its original parent")
}

func TestCloudRunJobsSpanModifier_MixedTraceIDBitWidths_64BitFirst(t *testing.T) {
	// Test the reverse case: 64-bit trace ID span arrives before 128-bit span.
	// The job span adopts the 64-bit ID first, then a 128-bit span from the
	// same trace arrives. The job span should be upgraded to 128-bit.
	// https://docs.datadoghq.com/tracing/guide/span_and_trace_id_format/

	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// First span arrives with only 64-bit trace ID (no high bits)
	rootSpan := InitSpan("user-service", "root.operation", "root-resource", "web", time.Now().UnixNano(), map[string]string{})
	rootSpan.ParentID = 0
	// No high bits

	// Second span from same trace arrives with 128-bit trace ID (has high bits)
	childSpan := InitSpan("user-service", "child.operation", "child-resource", "web", time.Now().UnixNano(), map[string]string{})
	childSpan.TraceID = rootSpan.TraceID // Same low 64 bits
	childSpan.ParentID = rootSpan.SpanID
	traceutil.SetTraceIDHigh(childSpan, "6958127700000000")

	// Modify both spans (64-bit first, then 128-bit)
	modifier.ModifySpan(nil, rootSpan)

	// After first span: job span should NOT have high bits yet
	assert.False(t, traceutil.HasTraceIDHigh(jobSpan), "Job span should not have high bits after 64-bit span")

	modifier.ModifySpan(nil, childSpan)

	// After second span: job span should be upgraded to 128-bit
	tidHigh, ok := traceutil.GetTraceIDHigh(jobSpan)
	assert.True(t, ok, "Job span should have high bits after upgrade")
	assert.Equal(t, "6958127700000000", tidHigh,
		"Job span should be upgraded to 128-bit when we see high bits later")

	// Verify: job span matches both spans
	assert.True(t, traceutil.SameTraceID(jobSpan, rootSpan), "Job span should match root span")
	assert.True(t, traceutil.SameTraceID(jobSpan, childSpan), "Job span should match child span")

	// Verify hierarchy preserved
	assert.Equal(t, jobSpan.SpanID, rootSpan.ParentID, "Root span should be reparented under job span")
	assert.Equal(t, rootSpan.SpanID, childSpan.ParentID, "Child span should keep its original parent")
}

func TestCloudRunJobsSpanModifier_NoUpgradeFromDifferentTrace(t *testing.T) {
	// Test that we don't accidentally upgrade the job span's trace ID with high bits
	// from a completely different trace.

	jobSpan := InitSpan("gcp.run.job", "gcp.run.job.task", "my-job", "serverless", time.Now().UnixNano(), map[string]string{})
	modifier := NewCloudRunJobsSpanModifier(jobSpan)

	// First span: 64-bit trace ID (trace A)
	traceASpan := InitSpan("user-service", "operation-a", "resource-a", "web", time.Now().UnixNano(), map[string]string{})
	traceASpan.ParentID = 0
	// No high bits

	// Second span: 128-bit trace ID from a DIFFERENT trace (trace B)
	traceBSpan := InitSpan("user-service", "operation-b", "resource-b", "web", time.Now().UnixNano(), map[string]string{})
	traceBSpan.ParentID = 0
	traceutil.SetTraceIDHigh(traceBSpan, "6958127700000000")

	// Ensure they have different low 64 bits
	assert.NotEqual(t, traceASpan.TraceID, traceBSpan.TraceID, "Test setup: traces should have different low 64 bits")

	// Modify both spans
	modifier.ModifySpan(nil, traceASpan)
	modifier.ModifySpan(nil, traceBSpan)

	// Verify: job span adopted trace A's ID and was NOT upgraded with trace B's high bits
	assert.Equal(t, traceASpan.TraceID, jobSpan.TraceID, "Job span should have trace A's low 64 bits")
	assert.False(t, traceutil.HasTraceIDHigh(jobSpan), "Job span should NOT have high bits from trace B")

	// Verify: only trace A span was reparented
	assert.Equal(t, jobSpan.SpanID, traceASpan.ParentID, "Trace A span should be reparented")
	assert.Equal(t, uint64(0), traceBSpan.ParentID, "Trace B span should NOT be reparented")
}
