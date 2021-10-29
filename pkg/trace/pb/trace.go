// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pb

//go:generate go run github.com/tinylib/msgp -file=span.pb.go -o span_gen.go -io=false
//go:generate go run github.com/tinylib/msgp -file=tracer_payload.pb.go -o tracer_payload_gen.go -io=false
//go:generate go run github.com/tinylib/msgp -io=false

// Trace is a collection of spans with the same trace ID
type Trace []*Span

// Traces is a list of traces. This model matters as this is what we unpack from msgp.
type Traces []Trace

// RemoveChunk removes a chunk by its index
func (p *TracerPayload) RemoveChunk(i int) {
	if i < 0 || i >= len(p.Chunks) {
		return
	}
	p.Chunks[i] = p.Chunks[len(p.Chunks)-1]
	p.Chunks = p.Chunks[:len(p.Chunks)-1]
}

// Split splits a tracer payload into two tracer payloads
// The first tracer payload will contain [0, i - 1] chunks,
// and the second tracer payload will contain [i, len - 1] chunks.
func (p *TracerPayload) Split(i int) (*TracerPayload, *TracerPayload) {
	if i < 0 {
		i = 0
	}
	if i > len(p.Chunks) {
		i = len(p.Chunks)
	}
	new := *p
	new.Chunks = p.Chunks[:i]
	p.Chunks = p.Chunks[i:]

	return &new, p
}
