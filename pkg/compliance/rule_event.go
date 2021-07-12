// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package compliance

// KVMap defines a key value map for storing attributes of a reported rule event
type KVMap map[string]string

// RuleEvent describes a log event sent for an evaluated compliance rule.
type RuleEvent struct {
	RuleID       string   `json:"rule_id,omitempty"`
	Framework    string   `json:"framework,omitempty"`
	Version      string   `json:"version,omitempty"`
	ResourceID   string   `json:"resource_id,omitempty"`
	ResourceType string   `json:"resource_type,omitempty"`
	Tags         []string `json:"tags"`
	Data         KVMap    `json:"data,omitempty"`
}
