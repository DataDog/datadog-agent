// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logshosttagsmissing

import (
	"os"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

// Check detects when the agent is collecting logs but no host-level tags are configured.
//
// The issue is triggered when all three conditions hold:
//  1. Logs collection is enabled (DD_LOGS_ENABLED=true).
//  2. The agent is effectively running in logs-only mode: metric payloads are disabled
//     (DD_ENABLE_PAYLOADS_SERIES=false AND DD_ENABLE_PAYLOADS_EVENTS=false) OR the agent
//     is configured to route logs to a dedicated endpoint while the main dd_url is absent.
//  3. No host-level tags are configured (DD_TAGS is unset or empty).
//
// In logs-only deployments host tags must be set explicitly — they are not derived from the
// metrics infrastructure the way they are in full-agent mode.
func Check() (*healthplatform.IssueReport, error) {
	// Condition 1: logs must be enabled.
	if !logsEnabled() {
		return nil, nil
	}

	// Condition 2: agent must be running in logs-only mode.
	if !logsOnlyMode() {
		return nil, nil
	}

	// Condition 3: no host tags configured.
	if tagsConfigured() {
		return nil, nil
	}

	return &healthplatform.IssueReport{
		IssueId: IssueID,
		Context: map[string]string{
			"logsEnabled":    "true",
			"tagsConfigured": "false",
		},
		Tags: []string{"logs", "tags", "configuration", "logs-only", "host-tags"},
	}, nil
}

// logsEnabled returns true when DD_LOGS_ENABLED is set to a truthy value.
func logsEnabled() bool {
	val, ok := os.LookupEnv("DD_LOGS_ENABLED")
	if !ok {
		return false
	}
	return isTruthy(val)
}

// logsOnlyMode returns true when the agent is configured for logs-only operation.
// This is detected by either:
//   - Both DD_ENABLE_PAYLOADS_SERIES and DD_ENABLE_PAYLOADS_EVENTS explicitly set to false, or
//   - DD_URL is unset while DD_LOGS_CONFIG_LOGS_DD_URL is set (dedicated logs endpoint, no main intake).
func logsOnlyMode() bool {
	seriesVal, seriesSet := os.LookupEnv("DD_ENABLE_PAYLOADS_SERIES")
	eventsVal, eventsSet := os.LookupEnv("DD_ENABLE_PAYLOADS_EVENTS")
	if seriesSet && eventsSet && isFalsy(seriesVal) && isFalsy(eventsVal) {
		return true
	}

	_, ddURLSet := os.LookupEnv("DD_URL")
	logsDDURL, logsDDURLSet := os.LookupEnv("DD_LOGS_CONFIG_LOGS_DD_URL")
	if !ddURLSet && logsDDURLSet && strings.TrimSpace(logsDDURL) != "" {
		return true
	}

	return false
}

// tagsConfigured returns true when at least one host tag is present in DD_TAGS.
func tagsConfigured() bool {
	val, ok := os.LookupEnv("DD_TAGS")
	if !ok {
		return false
	}
	return strings.TrimSpace(val) != ""
}

// isTruthy returns true for "1", "true", "yes" (case-insensitive).
func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// isFalsy returns true for "0", "false", "no" (case-insensitive).
func isFalsy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "0", "false", "no":
		return true
	}
	return false
}
