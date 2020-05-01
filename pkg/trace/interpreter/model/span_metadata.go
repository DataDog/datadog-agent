package model

import "github.com/StackVista/stackstate-agent/pkg/trace/pb"

// SpanMetadata contains the fields of the span meta that we are interested in
type SpanMetadata struct {
	ServiceName string
	CreateTime  int64
	Hostname    string
	PID         int
	Type        string
	Kind        string
}

// SpanWithMeta contains the span as well as the extracted meta data
type SpanWithMeta struct {
	*pb.Span
	*SpanMetadata
}
