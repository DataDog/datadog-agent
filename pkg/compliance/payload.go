// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package compliance

// RuleEvent describes a log event sent for an evaluated compliance rule.
type RuleEvent struct {
	RuleName     string            `json:"rule_name,omitempty"`
	RuleVersion  string            `json:"rule_version,omitempty"`
	Framework    string            `json:"framework,omitempty"`
	ResourceID   string            `json:"resource_id,omitempty"`
	ResourceType string            `json:"resource_type,omitempty"`
	Data         map[string]string `json:"data,omitempty"`
}
