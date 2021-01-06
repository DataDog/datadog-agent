//go:generate go run github.com/mailru/easyjson/easyjson $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package module

// AgentContext serializes the agent context to JSON
// easyjson:json
type AgentContext struct {
	RuleID        string   `json:"rule_id"`
	Tags          []string `json:"tags"`
	PolicyName    string   `json:"policy_name"`
	PolicyVersion string   `json:"policy_version"`
}

// Signal - Rule event wrapper used to send an event to the backend
// easyjson:json
type Signal struct {
	*AgentContext `json:"agent"`
	Title         string `json:"title"`
	Msg           string `json:"msg,omitempty"`
}
