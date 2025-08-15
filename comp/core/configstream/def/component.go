// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configstream implements a component to handle streaming configuration events to subscribers.
package configstream

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// team: agent-metric-pipelines

// Component is the component type.
type Component interface {
	// Subscribe returns a channel that streams configuration events, starting with a snapshot.
	// It also returns an unsubscribe function that must be called to clean up.
	Subscribe(req *pb.ConfigStreamRequest) (<-chan *pb.ConfigEvent, func())
}
