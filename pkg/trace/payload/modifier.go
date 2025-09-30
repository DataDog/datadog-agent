// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package payload defines common trace payload interfaces and types
package payload

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

// TracerPayloadModifier is an interface that allows tracer implementations to
// modify a TracerPayload as it is processed in the Agent's Process method.
type TracerPayloadModifier interface {
	Modify(*pb.TracerPayload)
}
