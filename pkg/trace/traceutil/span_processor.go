// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// SpanProcessor is the interface to apply extra logic on span after they are received by the agent
type SpanProcessor interface {
	// Process applies extra logic to the given span
	Process(tags map[string]string, span *pb.Span)
}
