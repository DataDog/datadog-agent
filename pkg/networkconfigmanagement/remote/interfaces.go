// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remote provides interfaces for remote device communications (SSH/Telnet) to retrieve configurations
package remote

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// Connector is an interface that can connect to a device execute commands on a device
type Connector interface {
	Connect() (Connection, error)
}

// Connection is an active connection that can fetch data from a device
type Connection interface {
	SetProfile(p *profile.NCMProfile)
	RetrieveRunningConfig(ctx context.Context) (*types.CommandResult, error)
	RetrieveStartupConfig(ctx context.Context) (*types.CommandResult, error)
	Verify(ctx context.Context) error
	// PushConfig pushes a new config to the device, returning a PushResult that
	// documents what commands were executed and their outputs. Note: If any
	// commands fail, PushConfig returns an error but ALSO returns a PushResult
	// with more details - calling code should not assume that the PushResult is
	// nil just because there was an error.
	PushConfig(ctx context.Context, config string) (*types.PushResult, types.RollbackError)
	Close() error
}
