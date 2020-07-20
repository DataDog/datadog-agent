package rules

import "github.com/DataDog/datadog-agent/pkg/security/secl/eval"

// RuleEvent - Rule event wrapper used to send an event to the backend
type RuleEvent struct {
	RuleID string     `json:"rule_id"`
	Event  eval.Event `json:"event"`
}
