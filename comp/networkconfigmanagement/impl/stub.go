// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !ncm

// Package networkconfigmanagementimpl implements a stub component when ncm is disabled.
package networkconfigmanagementimpl

import (
	"net/http"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
)

// Requires defines the dependencies for the networkconfigmanagement component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Logger    log.Component
}

// Provides defines the output of the networkconfigmanagement component
type Provides struct {
	Comp     networkconfigmanagement.Component
	Endpoint api.EndpointProvider `group:"agent_endpoint"`
}

type stubImpl struct{}

// NewComponent creates a stub networkconfigmanagement component
func NewComponent(reqs Requires) (Provides, error) {
	provides := Provides{
		Comp: nil,
		Endpoint: api.NewAgentEndpointProvider(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error": "ncm not enabled for agent"}`, http.StatusBadRequest)
		}, "/agent/ncm/config", "GET").Provider,
	}
	return provides, nil
}
