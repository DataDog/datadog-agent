// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

import (
	"encoding/json"
	"net/http"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// handleHealthIssues returns all detected health issues
func (h *healthPlatformImpl) handleHealthIssues(w http.ResponseWriter, _ *http.Request) {
	count, issues := h.GetAllIssues()

	response := map[string]interface{}{
		"count":  count,
		"issues": issues,
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		h.log.Warnf("Error marshalling health issues: %v", err)
		httputils.SetJSONError(w, err, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResponse)
}

// CheckResult represents the result of a health check run
type CheckResult struct {
	CheckID   string                `json:"check_id"`
	CheckName string                `json:"check_name"`
	IssueID   string                `json:"issue_id,omitempty"`
	Status    string                `json:"status"`
	Issue     *healthplatform.Issue `json:"issue,omitempty"`
}

// handleHealthDetect runs all registered health checks and returns results
func (h *healthPlatformImpl) handleHealthDetect(w http.ResponseWriter, _ *http.Request) {
	h.checksMux.RLock()
	defer h.checksMux.RUnlock()

	results := make([]CheckResult, 0, len(h.periodicChecks))

	for checkID, check := range h.periodicChecks {
		// Run the check function
		issueID, context := check.checkFunc()

		status := "healthy"
		var issue *healthplatform.Issue

		if issueID != "" {
			status = "issue_detected"
			// Build the full issue using the registry
			if builtIssue, err := h.remediationRegistry.BuildIssue(issueID, context); err == nil {
				issue = builtIssue
			} else {
				h.log.Warnf("Failed to build issue %s for check %s: %v", issueID, checkID, err)
			}
		}

		results = append(results, CheckResult{
			CheckID:   checkID,
			CheckName: check.checkName,
			IssueID:   issueID,
			Status:    status,
			Issue:     issue,
		})
	}

	response := map[string]interface{}{
		"results": results,
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		h.log.Warnf("Error marshalling health detection results: %v", err)
		httputils.SetJSONError(w, err, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResponse)
}
