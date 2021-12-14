// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

type serverlessSpanProcessor struct{}

// Process applies extra logic to the given span
func (s *serverlessSpanProcessor) Process(tags map[string]string, span *pb.Span) {
	if span.Service == "aws.lambda" && tags["service"] != "" {
		// service name could be incorrectly set to 'aws.lambda' in datadog lambda libraries
		span.Service = tags["service"]
	}
}
