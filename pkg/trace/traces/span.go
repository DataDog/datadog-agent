package traces

import "io"

type Span interface {
	TraceID() uint64
	SpanID() uint64

	UnsafeService() string
	SetService(s string)

	UnsafeName() string
	SetName(s string)

	UnsafeResource() string
	SetResource(s string)

	Duration() int64
	SetDuration(d int64)

	ParentID() uint64
	SetParentID(x uint64)

	Start() int64
	SetStart(x int64)

	UnsafeType() string
	SetType(s string)

	WriteProto(w io.Writer) error

	DebugString() string
}
