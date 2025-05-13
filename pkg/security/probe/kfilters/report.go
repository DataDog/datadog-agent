// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kfilters holds kfilters related files
package kfilters

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// AcceptModeRule describes a rule that is in accept mode
type AcceptModeRule struct {
	RuleID string `json:"rule_id"`
}

// ApproverReport describes the result of the kernel policy and the approvers for an event type
type ApproverReport struct {
	Mode            PolicyMode       `json:"mode"`
	Approvers       rules.Approvers  `json:"approvers,omitempty"`
	AcceptModeRules []AcceptModeRule `json:"accept_mode_rules,omitempty"`
}

// KernelFilterReport describes the event types and their associated policy policies
type KernelFilterReport struct {
	ApproverReports map[string]*ApproverReport `json:"approvers"`
}

// MarshalJSON marshals the KernelFilterReport to JSON
func (r *KernelFilterReport) MarshalJSON() ([]byte, error) {
	reports := make(map[string]json.RawMessage)

	for eventType, report := range r.ApproverReports {
		if (report.Mode == PolicyModeNoFilter || report.Mode == PolicyModeAccept) && len(report.AcceptModeRules) == 0 {
			continue
		}
		raw, err := json.Marshal(report)
		if err != nil {
			return nil, err
		}
		reports[eventType] = raw
	}

	return json.Marshal(reports)
}

// String returns a JSON representation of the KernelFilterReport
func (r *KernelFilterReport) String() string {
	content, _ := json.Marshal(r)
	return string(content)
}

// NewKernelFilterReport returns filtering policy applied per event type
func NewKernelFilterReport(config *config.Config, rs *rules.RuleSet) (*KernelFilterReport, error) {
	approverReports := make(map[eval.EventType]*ApproverReport)

	// We need to call the approver detection even when approvers aren't enabled as it may have impact on some rule flags and
	// the discarder mechanism, see ruleset.go
	approvers, rules, err := rs.GetApprovers(GetCapababilities())
	if err != nil {
		return nil, err
	}

	for _, eventType := range rs.GetEventTypes() {
		report := &ApproverReport{Mode: PolicyModeDeny}
		approverReports[eventType] = report

		if !config.EnableKernelFilters {
			report.Mode = PolicyModeNoFilter
			continue
		}

		if !config.EnableApprovers {
			report.Mode = PolicyModeAccept
			continue
		}

		if _, exists := allCapabilities[eventType]; !exists {
			report.Mode = PolicyModeAccept
			continue
		}

		if values, exists := approvers[eventType]; exists {
			report.Approvers = values
		} else {
			report.Mode = PolicyModeAccept
			if rule := rules[eventType]; rule != nil {
				report.AcceptModeRules = append(report.AcceptModeRules, AcceptModeRule{
					RuleID: rule.ID,
				})
			}
		}
	}

	return &KernelFilterReport{ApproverReports: approverReports}, nil
}
