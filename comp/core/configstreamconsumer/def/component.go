// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configstreamconsumer implements a component that consumes config streams from the core agent.
//
// team: agent-metric-pipelines agent-configuration
package configstreamconsumer

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// Component is the config stream consumer component interface.
type Component interface {
	// Start initiates the config stream connection and processing loop.
	// It should be called during the OnStart lifecycle hook.
	Start(ctx context.Context) error

	// WaitReady blocks until the first config snapshot has been received and applied.
	// This ensures the consumer has a consistent config view before proceeding.
	WaitReady(ctx context.Context) error

	// Reader returns a config reader backed by the streamed configuration.
	// This provides a model.Reader interface for accessing config values.
	Reader() model.Reader

	// Subscribe returns a channel that receives config change events.
	// The returned unsubscribe function should be called to clean up.
	Subscribe() (<-chan ChangeEvent, func())
}

// ChangeEvent represents a configuration change event.
type ChangeEvent struct {
	// Key is the configuration key that changed
	Key string
	// OldValue is the previous value (nil if newly set)
	OldValue interface{}
	// NewValue is the new value (nil if deleted)
	NewValue interface{}
}
