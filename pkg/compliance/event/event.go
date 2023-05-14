// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import "time"

const (
	// Passed is used to report successful result of a rule check (condition passed)
	Passed = "passed"
	// Failed is used to report unsuccessful result of a rule check (condition failed)
	Failed = "failed"
	// Error is used to report result of a rule check that resulted in an error (unable to evaluate condition)
	Error = "error"
)

// Data defines a key value map for storing attributes of a reported rule event
type Data map[string]interface{}

// Event describes a log event sent for an evaluated compliance/security rule.
type Event struct {
	AgentRuleID      string      `json:"agent_rule_id,omitempty"`
	AgentRuleVersion int         `json:"agent_rule_version,omitempty"`
	AgentFrameworkID string      `json:"agent_framework_id,omitempty"`
	AgentVersion     string      `json:"agent_version,omitempty"`
	Result           string      `json:"result,omitempty"`
	ResourceType     string      `json:"resource_type,omitempty"`
	ResourceID       string      `json:"resource_id,omitempty"`
	Tags             []string    `json:"tags"`
	Data             interface{} `json:"data,omitempty"`
	ExpireAt         time.Time   `json:"expire_at,omitempty"`
	Evaluator        string      `json:"evaluator,omitempty"`
}
