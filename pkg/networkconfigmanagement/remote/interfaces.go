// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remote provides interfaces for remote device communications (SSH/Telnet) to retrieve configurations
package remote

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

// Connector is an interface that can connect to a device execute commands on a device
type Connector interface {
	Connect() (Connection, error)
}

// Connection is an active connection that can fetch data from a device
type Connection interface {
	SetProfile(p *profile.NCMProfile)
	RetrieveRunningConfig(ctx context.Context) ([]byte, error)
	RetrieveStartupConfig(ctx context.Context) ([]byte, error)
	Verify(ctx context.Context) error
	PushConfig(ctx context.Context, config string) error
	Close() error
}
