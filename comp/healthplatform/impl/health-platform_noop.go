// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

import (
	"encoding/json"
	"net/http"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

// noopHealthPlatform is a no-op implementation of the health platform component
// Used when the health platform is disabled via configuration
type noopHealthPlatform struct{}

// ReportIssue does nothing when the health platform is disabled
func (n *noopHealthPlatform) ReportIssue(_ string, _ string, _ *healthplatformpayload.IssueReport) error {
	return nil
}

// RegisterCheck does nothing when the health platform is disabled
func (n *noopHealthPlatform) RegisterCheck(_ string, _ string, _ healthplatform.HealthCheckFunc, _ time.Duration) error {
	return nil
}

// GetAllIssues returns empty results when the health platform is disabled
func (n *noopHealthPlatform) GetAllIssues() (int, map[string]*healthplatformpayload.Issue) {
	return 0, make(map[string]*healthplatformpayload.Issue)
}

// GetIssueForCheck returns nil when the health platform is disabled
func (n *noopHealthPlatform) GetIssueForCheck(_ string) *healthplatformpayload.Issue {
	return nil
}

// ClearIssuesForCheck does nothing when the health platform is disabled
func (n *noopHealthPlatform) ClearIssuesForCheck(_ string) {
}

// ClearAllIssues does nothing when the health platform is disabled
func (n *noopHealthPlatform) ClearAllIssues() {
}

// ============================================================================
// HTTP API Handlers (noop)
// ============================================================================

// getIssuesHandler handles GET /health-platform/issues when disabled
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

// fillFlare does nothing when the health platform is disabled (no file created)
func (n *noopHealthPlatform) fillFlare(_ flaretypes.FlareBuilder) error {
	return nil
}
