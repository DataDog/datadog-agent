// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
)

// Payload specifies information about a set of traces received by the API.
type Payload struct {
	// Source specifies information about the source of these traces, such as:
	// language, interpreter, tracer version, etc.
	Source *info.TagStats

	// TracerPayload holds the incoming payload from the tracer.
	TracerPayload *pb.TracerPayload

	// ClientComputedTopLevel specifies that the client has already marked top-level
	// spans.
	ClientComputedTopLevel bool

	// ClientComputedStats reports whether the client has computed and sent over stats
	// so that the agent doesn't have to.
	ClientComputedStats bool

	// ClientDroppedP0s specifies the number of P0 traces chunks dropped by the client.
	ClientDroppedP0s int64

	// ProcessTags is a list of tags describing an instrumented process.
	ProcessTags string

	// ContainerTags is a list of tags describing the container we received this payload from
	ContainerTags []string
}

// Chunks returns chunks in TracerPayload
func (p *Payload) Chunks() []*pb.TraceChunk {
	return p.TracerPayload.Chunks
}

// Chunk returns a chunk in TracerPayload by its index
func (p *Payload) Chunk(i int) *pb.TraceChunk {
	return p.TracerPayload.Chunks[i]
}

// RemoveChunk removes a chunk in TracerPayload by its index
func (p *Payload) RemoveChunk(i int) {
	p.TracerPayload.RemoveChunk(i)
}

// ReplaceChunk replaces a chunk in TracerPayload at a given index
func (p *Payload) ReplaceChunk(i int, chunk *pb.TraceChunk) {
	p.TracerPayload.Chunks[i] = chunk
}

// PayloadV1 specifies information about a set of traces received by the V1 API.
type PayloadV1 struct {
	// Source specifies information about the source of these traces, such as:
	// language, interpreter, tracer version, etc.
	Source *info.TagStats

	// TracerPayload holds the incoming payload from the tracer.
	TracerPayload *idx.InternalTracerPayload

	// ClientComputedTopLevel specifies that the client has already marked top-level
	// spans.
	ClientComputedTopLevel bool

	// ClientComputedStats reports whether the client has computed and sent over stats
	// so that the agent doesn't have to.
	ClientComputedStats bool

	// ClientDroppedP0s specifies the number of P0 traces chunks dropped by the client.
	ClientDroppedP0s int64

	// ProcessTags is a list of tags describing an instrumented process.
	ProcessTags string

	// ContainerTags is a list of tags describing the container we received this payload from
	ContainerTags []string
}
