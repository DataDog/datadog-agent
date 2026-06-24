// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatformnoopimpl provides a noop implementation of the health-platform component
package healthplatformnoopimpl

import (
	"context"
	"encoding/json"
	"net/http"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// Provides defines the output of the noop health-platform component
type Provides struct {
	compdef.Out
	Comp          healthplatformdef.Component
	APIGetIssues  api.AgentEndpointProvider
	FlareProvider flaretypes.Provider
}

// noopHealthPlatform is a no-op implementation of the health platform component
type noopHealthPlatform struct{}

// NewComponent creates a noop health platform component with zero dependencies.
// Used by commands that don't need health reporting (analyzelogs, jmx, etc.).
func NewComponent() Provides {
	noop := &noopHealthPlatform{}
	return Provides{
		Comp:          noop,
		APIGetIssues:  api.NewAgentEndpointProvider(noop.getIssuesHandler, "/health-platform/issues", "GET"),
		FlareProvider: flaretypes.NewProvider(noop.fillFlare),
	}
}

func (n *noopHealthPlatform) ReportIssue(_ *healthplatformpayload.Issue) error {
	return nil
}

func (n *noopHealthPlatform) GetAllIssues() (int, map[string]*healthplatformpayload.Issue) {
	return 0, make(map[string]*healthplatformpayload.Issue)
}

func (n *noopHealthPlatform) GetIssue(_ string) *healthplatformpayload.Issue {
	return nil
}

func (n *noopHealthPlatform) RegisterIssuesObserver(_ healthplatformdef.IssuesObserver) {}

func (n *noopHealthPlatform) ResolveIssue(_ string) {
}

func (n *noopHealthPlatform) ResolveAllIssues() {
}

func (n *noopHealthPlatform) GetActiveIssueIDsByIssueName(_ string) []string {
	return nil
}

func (n *noopHealthPlatform) getIssuesHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := struct {
		Count  int                                     `json:"count"`
		Issues map[string]*healthplatformpayload.Issue `json:"issues"`
	}{
		Count:  0,
		Issues: make(map[string]*healthplatformpayload.Issue),
	}
	_ = json.NewEncoder(w).Encode(response)
}

func (n *noopHealthPlatform) fillFlare(_ context.Context, _ flaretypes.FlareBuilder) error {
	return nil
}
