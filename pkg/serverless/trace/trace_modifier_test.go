package trace

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func TestFormatTraceForCloudRunWithValidRootSpan(t *testing.T) {
	spans := []*pb.Span{
		{
			SpanID:   1,
			ParentID: 123,
		},
		{
			SpanID:   2,
			ParentID: 124,
		},
		{
			SpanID:   3,
			ParentID: 0,
		},
	}

	spans = formatTraceForCloudRun(spans)

	assert.Len(t, spans, 4)

	oldRootSpan := spans[2]
	newRootSpan := spans[3]

	assert.Equal(t, "gcp.cloudrun", newRootSpan.Name)
	assert.Equal(t, "gcp.cloudrun", newRootSpan.Resource)
	assert.Equal(t, uint64(0), newRootSpan.ParentID)
	assert.Equal(t, newRootSpan.SpanID, oldRootSpan.ParentID)
	assert.Equal(t, newRootSpan.TraceID, oldRootSpan.TraceID)
}

func TestFormatTraceForCloudRunWithNoRootSpan(t *testing.T) {
	spans := []*pb.Span{
		{
			SpanID:   1,
			ParentID: 123,
		},
		{
			SpanID:   2,
			ParentID: 124,
		},
		{
			SpanID:   3,
			ParentID: 125,
		},
	}

	spans = formatTraceForCloudRun(spans)

	assert.Len(t, spans, 3)
}
