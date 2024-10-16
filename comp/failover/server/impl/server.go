// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package serverimpl implements the server component interface
package serverimpl

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	server "github.com/DataDog/datadog-agent/comp/failover/server/def"
)

// Requires defines the dependencies for the server component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle

	Logger log.Component
}

// Provides defines the output of the server component
type Provides struct {
	Comp server.Component
}

// NewComponent creates a new server component
func NewComponent(reqs Requires) (Provides, error) {

	reqs.Logger.Info("Start Failover Server")
	startNewApiServer()

	// TODO: Implement the server component
	provides := Provides{}
	return provides, nil
}
