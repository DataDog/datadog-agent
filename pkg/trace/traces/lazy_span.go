package traces

// TODO: Implement me with molecule.
type LazySpan struct {
	raw []byte
}

func NewLazySpan(raw []byte) *LazySpan {
	return &LazySpan{
		raw: raw,
	}
}

func (l *LazySpan) UnsafeTraceID() string {
	return "TODO"
}

func (l *LazySpan) SpanID() uint64 {
	// TODO
	return 0
}

func (l *LazySpan) DebugString() string {
	return "TODO"
}
