// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package noopimpl provides a no-op implementation of the health platform component.
// It is intentionally lightweight — it imports only core/def and agent-payload types
// so that callers do not transitively depend on the full core/impl package.
package noopimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// NoopHealthPlatform is a no-op implementation of the health platform component.
// Used when the health platform is disabled via configuration.
type NoopHealthPlatform struct{}

// NewNoopComponent creates a no-op health platform component (disabled state).
// It satisfies the healthplatform.Component interface but performs no operations.
func NewNoopComponent() healthplatform.Component {
	return &NoopHealthPlatform{}
}

// NewNoopHealthPlatform returns a concrete *NoopHealthPlatform for callers that need
// direct access to GetIssuesHandler and FillFlare (e.g. core/impl.NewComponent).
func NewNoopHealthPlatform() *NoopHealthPlatform {
	return &NoopHealthPlatform{}
}

// ReportIssue does nothing when the health platform is disabled.
func (n *NoopHealthPlatform) ReportIssue(_ string, _ string, _ *healthplatformpayload.IssueReport) error {
	return nil
}

// RegisterCheck does nothing when the health platform is disabled.
func (n *NoopHealthPlatform) RegisterCheck(_ string, _ string, _ healthplatform.HealthCheckFunc, _ time.Duration) error {
	return nil
}

// GetAllIssues returns empty results when the health platform is disabled.
func (n *NoopHealthPlatform) GetAllIssues() (int, map[string]*healthplatformpayload.Issue) {
	return 0, make(map[string]*healthplatformpayload.Issue)
}

// GetIssueForCheck returns nil when the health platform is disabled.
func (n *NoopHealthPlatform) GetIssueForCheck(_ string) *healthplatformpayload.Issue {
	return nil
}

// ClearIssuesForCheck does nothing when the health platform is disabled.
func (n *NoopHealthPlatform) ClearIssuesForCheck(_ string) {
}

// ClearAllIssues does nothing when the health platform is disabled.
func (n *NoopHealthPlatform) ClearAllIssues() {
}

// GetIssuesHandler handles GET /health-platform/issues when disabled.
func (n *NoopHealthPlatform) GetIssuesHandler(w http.ResponseWriter, _ *http.Request) {
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

// FillFlare does nothing when the health platform is disabled (no file created).
func (n *NoopHealthPlatform) FillFlare(_ context.Context, _ flaretypes.FlareBuilder) error {
	return nil
}
