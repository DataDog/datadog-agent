// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package store

// IssueReport is the agent-internal contract for reporting a health issue
// to the health platform store.
//
// IssueId uniquely identifies this issue *instance* — it is the key in the
// store's in-memory map. Two ReportIssue calls with the same IssueId update
// the same instance (new → ongoing state machine). Different instances of the
// same issue type (e.g. the same database problem on two different hosts) must
// use different IssueIds.
//
// IssueType is the template identifier. The issue registry looks it up to fill
// in the human-readable title, severity, remediation steps, etc.
type IssueReport struct {
	// IssueId is the unique instance id, used as the store's map key.
	// Examples:
	//   "check-execution-failure:mysql:0123abcd"
	//   "ad-template:redis:svc-foo:deadbeef"
	//   "db-not-reachable:mysql-prod-1"
	IssueID string

	// IssueType is the template id looked up in the issue registry.
	// Examples: "check-execution-failure", "ad-misconfiguration"
	IssueType string

	// Source is the reporting integration or component name.
	// Examples: "mysql", "autodiscovery", "docker"
	Source string

	// Context provides variables for filling in the issue template.
	Context map[string]string

	// Tags are appended to the template's default tags.
	Tags []string
}
