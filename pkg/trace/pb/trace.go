package pb

// Trace is a collection of spans with the same trace ID
type Trace []*Span

// Traces is a list of traces. This model matters as this is what we unpack from msgp.
type Traces []Trace
