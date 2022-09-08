package trace

import (
	"math/rand"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// ModifyTrace takes in a trace and modifies it in place to return a new trace
func ModifyTrace(trace *[]*pb.Span) {
	// For now, let's just assume we're in Cloud Run
	formatTraceForCloudRun(trace)
}

func formatTraceForCloudRun(trace *[]*pb.Span) {
	oldRoot := traceutil.GetRoot(*trace)
	// GetRoot returns the last span if we don't have a "true" root value.
	// If that's the case, we don't want to wrap the span.
	if oldRoot.ParentID != 0 {
		return
	}
	root := &pb.Span{
		TraceID:  oldRoot.TraceID,
		Name:     "gcp.cloudrun",
		Resource: "gcp.cloudrun",
		Start:    oldRoot.Start,
		SpanID:   rand.Uint64(),
		Duration: oldRoot.Duration,
		Error:    oldRoot.Error,
		Meta:     oldRoot.Meta,
		Type:     oldRoot.Type,
		Service:  oldRoot.Service,
	}
	oldRoot.ParentID = root.SpanID
	*trace = append(*trace, root)
}
