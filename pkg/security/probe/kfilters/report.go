// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kfilters

import (
	"encoding/json"
	"math"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// PolicyReport describes the result of the kernel policy and the approvers for an event type
type PolicyReport struct {
	Mode      PolicyMode
	Flags     PolicyFlag
	Approvers rules.Approvers
}

type PolicyReportToPrint struct {
	Mode      string
	Flags     []string
	Approvers ReportApprovers
}

type ApproverToPrint struct {
	Field string
	Value interface{}
	Type  string // rules.Approvers is int
}

type ReportApprovers map[string][]ApproverToPrint
type PoliciesByEventTypeToPrint map[string]*PolicyReportToPrint

// ApplyRuleSetReport describes the event types and their associated policy policies
type ApplyRuleSetReportToPrint struct {
	Policies map[string]*PolicyReportToPrint
}

// ApplyRuleSetReport describes the event types and their associated policy policies
type ApplyRuleSetReport struct {
	Policies map[string]*PolicyReport
}

// NewApplyRuleSetReport returns filtering policy applied per event type
func NewApplyRuleSetReport(config *config.Config, rs *rules.RuleSet) (*ApplyRuleSetReport, error) {
	policies := make(map[eval.EventType]*PolicyReport)

	approvers, err := rs.GetApprovers(GetCapababilities())
	if err != nil {
		return nil, err
	}

	for _, eventType := range rs.GetEventTypes() {
		report := &PolicyReport{Mode: PolicyModeDeny, Flags: math.MaxUint8}
		policies[eventType] = report

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
		}
	}

	return &ApplyRuleSetReport{Policies: policies}, nil
}

func NewPolicyReportToPrint() PolicyReportToPrint {
	var flags []string
	return PolicyReportToPrint{
		Mode:      "",
		Flags:     flags,
		Approvers: make(ReportApprovers),
	}
}

func (r *ApplyRuleSetReport) String() string {
	policies := make(map[eval.EventType]*PolicyReportToPrint)

	for eventType, policy := range r.Policies {
		policyReportToPrint := PolicyReportToPrint{
			Mode:  policy.Mode.String(),
			Flags: policy.Flags.StringArray(),
		}

		approversToPrint := make(map[eval.Field][]ApproverToPrint)

		for evalField, filterValues := range policy.Approvers {

			for _, filterValue := range filterValues {
				approversToPrint[evalField] = append(approversToPrint[evalField],
					ApproverToPrint{
						Field: filterValue.Field,
						Value: filterValue.Value,
						Type:  filterValue.Type.String(),
					},
				)
			}
		}
		policyReportToPrint.Approvers = approversToPrint
		policies[eventType] = &policyReportToPrint
	}

	wholeReport := &ApplyRuleSetReportToPrint{Policies: policies}

	content, _ := json.MarshalIndent(wholeReport, "", "\t")
	return string(content)
}
