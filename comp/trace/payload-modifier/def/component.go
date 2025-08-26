// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package payloadmodifier defines the trace payload modifier component interface
package payloadmodifier

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

// team: agent-apm

// Component provides trace payload modification functionality
type Component interface {
	// Modify modifies the given TracerPayload
	// No-op if payload modification is not enabled
	Modify(*pb.TracerPayload)
}
