// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// Payload specifies information about a set of traces received by the API.
type Payload struct {
	// TracerPayload holds the incoming payload from the tracer.
	TracerPayload *pb.TracerPayload

	Meta Metadata
}

type Metadata struct {
	// Source specifies information about the source of these traces, such as:
	// language, interpreter, tracer version, etc.
	Source *info.TagStats

	// ClientComputedTopLevel specifies that the client has already marked top-level
	// spans.
	ClientComputedTopLevel bool

	// ClientComputedStats reports whether the client has computed and sent over stats
	// so that the agent doesn't have to.
	ClientComputedStats bool

	// ClientDroppedP0s specifies the number of P0 traces chunks dropped by the client.
	ClientDroppedP0s int64
}

