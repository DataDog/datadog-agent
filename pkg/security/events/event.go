//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=readonly -no_std_marshalers $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package events holds events related files
package events

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// AgentContext serializes the agent context to JSON
// easyjson:json
type AgentContext struct {
	RuleID        string            `json:"rule_id"`
	RuleVersion   string            `json:"rule_version,omitempty"`
	RuleActions   []json.RawMessage `json:"rule_actions,omitempty"`
	PolicyName    string            `json:"policy_name,omitempty"`
	PolicyVersion string            `json:"policy_version,omitempty"`
	Version       string            `json:"version,omitempty"`
	OS            string            `json:"os,omitempty"`
	Arch          string            `json:"arch,omitempty"`
	Origin        string            `json:"origin,omitempty"`
	KernelVersion string            `json:"kernel_version,omitempty"`
	Distribution  string            `json:"distribution,omitempty"`
}

// BackendEvent - Rule event wrapper used to send an event to the backend
// easyjson:json
type BackendEvent struct {
	AgentContext `json:"agent"`
	Title        string `json:"title"`
}

// Event is the interface that an event must implement to be sent to the backend
type Event interface {
	GetWorkloadID() string
	GetTags() []string
	GetType() string
	GetActionReports() []model.ActionReport
	GetFieldValue(eval.Field) (interface{}, error)
}

// EventSender defines an event sender
type EventSender interface {
	SendEvent(rule *rules.Rule, event Event, extTagsCb func() ([]string, bool), service string)
}
