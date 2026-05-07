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
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
)

// Requires defines the dependencies for the networkconfigmanagement component
type Requires struct{}

// Provides defines the output of the networkconfigmanagement component
type Provides struct {
	Comp     networkconfigmanagement.Component
	Endpoint api.EndpointProvider `group:"agent_endpoint"`
}

// NewComponent creates a stub networkconfigmanagement component
func NewComponent(_ Requires) (Provides, error) {
	provides := Provides{
		Comp: nil,
		Endpoint: api.NewAgentEndpointProvider(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"error": "ncm not enabled for agent"}`, http.StatusBadRequest)
		}, "/ncm/config", "GET").Provider,
	}
	return provides, nil
}
