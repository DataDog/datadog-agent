// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ndmsyslogsimpl implements the ndmsyslogs component interface
package ndmsyslogsimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	ndmsyslogs "github.com/DataDog/datadog-agent/comp/ndmsyslogs/def"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the ndmsyslogs component
type Requires struct {
	Lifecycle   compdef.Lifecycle
	Hostname    hostnameinterface.Component
	Config      pkgconfigmodel.Reader
	Compression logscompression.Component
}

// Provides defines the output of the ndmsyslogs component
type Provides struct {
	Comp ndmsyslogs.Component
}

// ndmsyslogsComponent implements the ndmsyslogs.Component interface
type ndmsyslogsComponent struct {
	tcpIntegration *CustomTCPIntegration
}

// Start starts the NDM syslogs listener
func (n *ndmsyslogsComponent) Start() error {
	return n.tcpIntegration.Start()
}

// Stop stops the NDM syslogs listener
func (n *ndmsyslogsComponent) Stop() {
	n.tcpIntegration.Stop()
}

// NewComponent creates a new ndmsyslogs component
func NewComponent(reqs Requires) (Provides, error) {
	// Create the custom TCP integration for NDM syslogs
	tcpIntegration := NewCustomTCPIntegration(
		1234,              // Default port for NDM syslogs
		"tag:ndm-syslogs", // Custom tag for NDM syslogs
		"https://ndm-syslogs-endpoint.example.com", // Custom endpoint for NDM syslogs
		reqs.Hostname,
		reqs.Config,
		reqs.Compression,
	)

	// Create the component
	component := &ndmsyslogsComponent{
		tcpIntegration: tcpIntegration,
	}

	// Set up lifecycle hooks
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("Starting NDM syslogs component")
			return component.Start()
		},
		OnStop: func(ctx context.Context) error {
			log.Info("Stopping NDM syslogs component")
			component.Stop()
			return nil
		},
	})

	provides := Provides{
		Comp: component,
	}
	return provides, nil
}
