// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package networkconfigmanagement provides the component for retrieving network device configurations.
package networkconfigmanagement

// team: ndm-integrations

import (
	"context"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// Component is the component type.
type Component interface {
	// RegisterDevice tells the component how to connect to a device.
	RegisterDevice(config *config.DeviceInstance) error
	// ReportConfig runs the NCM check - it fetches the running and startup
	// config and communicates them to the DD backend, along with an inventory
	// report if necessary.
	ReportConfig(ctx context.Context, deviceID string, sender sender.Sender) error
	// RollbackConfig rolls back a device to a previous configuration that's
	// saved locally on this agent.
	RollbackConfig(ctx context.Context, deviceID string, configVersion string, hash string) (*remote.PushResult, types.RollbackError)
	// SetMaxReportInterval sets a maximum time to wait between sending
	// inventory reports.
	SetMaxReportInterval(interval time.Duration)
	// GetConfigEndpointHandler returns an HTTP handler for getting configuration
	GetConfigEndpointHandler() http.HandlerFunc
	// RollbackEndpointHandler returns an HTTP handler for getting configuration
	RollbackEndpointHandler() http.HandlerFunc
}
