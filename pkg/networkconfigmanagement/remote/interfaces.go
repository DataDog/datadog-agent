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

// PushResult captures the possible outcomes of executing a PushConfig. We need
// a more complex structure than just a simple error because we want to track
// what commands were executed and which ones completed successfully - if, for
// example, a config push successfully copies the configuration to the device
// and sets the running config but fails to set the startup config, it's
// important that the calling code know that the running configuration has been
// changed even though the full config replace operation failed.
type PushResult struct {
	// CopyConfig holds the CommandResults of copying the configuration to the
	// device (generally via SCP)
	CopyConfig ResultList `json:"copy_config"`
	// SetRunning holds the results of attempting to set the device's running
	// config to the copied configuration.
	SetRunning ResultList `json:"set_running"`
	// SetStartup holds the results of attempting to set the device's startup
	// config to the copied configuration. In most profiles this is done by
	// copying the running config to the startup config, so this will be empty
	// if SetRunning contains errors
	SetStartup ResultList `json:"set_startup"`
}

// Connection is an active connection that can fetch data from a device
type Connection interface {
	SetProfile(p *profile.NCMProfile)
	RetrieveRunningConfig(ctx context.Context) (*CommandResult, error)
	RetrieveStartupConfig(ctx context.Context) (*CommandResult, error)
	Verify(ctx context.Context) error
	// PushConfig pushes a new config to the device, returning a PushResult that
	// documents what commands were executed and their outputs. Note: If any
	// commands fail, PushConfig returns an error but ALSO returns a PushResult
	// with more details - calling code should not assume that the PushResult is
	// nil just because there was an error.
	PushConfig(ctx context.Context, config string) (*PushResult, types.RollbackError)
	Close() error
}
