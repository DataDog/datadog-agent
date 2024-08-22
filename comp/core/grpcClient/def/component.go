// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package grpcClient ... /* TODO: detailed doc comment for the component */
package grpcClient

import (
	"context"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// team: /* TODO: add team name */

// Component is the component type.
type Component interface {
	pb.AgentSecureClient
	NewStreamContextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc)
	NewStreamContext() (context.Context, context.CancelFunc)
	Cancel()
	Context() context.Context
}
