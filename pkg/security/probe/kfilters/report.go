// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kfilters holds kfilters related files
package kfilters

import (
	"encoding/json"
	"slices"
	"sort"

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
	ApproversOnly   []eval.Field     `json:"approvers_only,omitempty"`
}

// FilterReport describes the event types and their associated policy policies
type FilterReport struct {
	ApproverReports  map[eval.EventType]*ApproverReport `json:"approvers,omitempty"`
	DiscardersReport *rules.DiscardersReport            `json:"discarders,omitempty"`
	Stats            *rules.ApproverStats               `json:"stats,omitempty"`
}

// MarshalJSON marshals the FilterReport to JSON
func (r *FilterReport) MarshalJSON() ([]byte, error) {
	approverReports := make(map[eval.EventType]json.RawMessage)

	for eventType, report := range r.ApproverReports {
		if (report.Mode == PolicyModeNoFilter || report.Mode == PolicyModeAccept) && len(report.AcceptModeRules) == 0 {
			continue
		}
		raw, err := json.Marshal(report)
		if err != nil {
			return nil, err
		}
		approverReports[eventType] = raw
	}

	report := struct {
		ApproverReports  map[eval.EventType]json.RawMessage `json:"approvers,omitempty"`
		DiscardersReport *rules.DiscardersReport            `json:"discarders,omitempty"`
		Stats            *rules.ApproverStats               `json:"stats,omitempty"`
	}{
		ApproverReports:  approverReports,
		DiscardersReport: r.DiscardersReport,
		Stats:            r.Stats,
	}

	return json.Marshal(report)
}

// String returns a JSON representation of the FilterReport
func (r *FilterReport) String() string {
	content, _ := json.Marshal(r)
	return string(content)
}

func computeApprovers(config *config.Config, rs *rules.RuleSet, capabilities map[eval.EventType]rules.FieldCapabilities) (*FilterReport, []*rules.Rule, error) {
	// get the approvers and accept mode rules
	approvers, stats, noDiscarderRules, err := rs.GetApprovers(capabilities)
	if err != nil {
		return nil, nil, err
	}

	filterReport := &FilterReport{
		ApproverReports:  make(map[eval.EventType]*ApproverReport),
		DiscardersReport: &rules.DiscardersReport{},
		Stats:            stats,
	}

	// generate the approver reports
	for _, eventType := range rs.GetEventTypes() {
		report := &ApproverReport{Mode: PolicyModeDeny}
		filterReport.ApproverReports[eventType] = report

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

			// report approvers that are marked as approver only
			for _, evtCapability := range capabilities[eventType] {
				for field := range values {
					if evtCapability.Field == field && evtCapability.FilterMode == rules.ApproverOnlyMode {
						if !slices.Contains(report.ApproversOnly, evtCapability.Field) {
							report.ApproversOnly = append(report.ApproversOnly, evtCapability.Field)
						}
					}
				}
			}
		} else {
			report.Mode = PolicyModeAccept
			if rule := stats.AcceptModeRules[eventType]; rule != nil {
				report.AcceptModeRules = append(report.AcceptModeRules, AcceptModeRule{
					RuleID: rule.ID,
				})
			}
		}
	}

	return filterReport, noDiscarderRules, nil
}

func ruleListToMap(rules []*rules.Rule) map[eval.RuleID]bool {
	m := make(map[eval.RuleID]bool, len(rules))
	for _, rule := range rules {
		m[rule.ID] = true
	}
	return m
}

// ComputeFilters computes the approver and discarder and returns a FilterReport
func ComputeFilters(config *config.Config, rs *rules.RuleSet) (*FilterReport, error) {
	computeFilters := func(rs *rules.RuleSet, capabilities map[eval.EventType]rules.FieldCapabilities) (*FilterReport, error) {
		filterReport, noDiscarderRules, err := computeApprovers(config, rs, capabilities)
		if err != nil {
			return nil, err
		}
		rs.WithExcludedRuleFromDiscarders(ruleListToMap(noDiscarderRules))

		discarderReport, err := rs.GetDiscardersReport()
		if err != nil {
			return nil, err
		}

		filterReport.DiscardersReport = discarderReport

		return filterReport, nil
	}

	var (
		filterReport *FilterReport
		err          error
	)

	// first attempt to compute the approvers and discarders
	filterReport, err = computeFilters(rs, allCapabilities)
	if err != nil {
		return nil, err
	}

	// if some invalid discarders, try to improve putting some approvers in ApproverOnly mode
	if len(filterReport.DiscardersReport.Invalid) > 0 {
		event := rs.NewEvent()

		for _, invalid := range filterReport.DiscardersReport.Invalid {
			eventType, _, _, err := event.GetFieldMetadata(invalid.Field)
			if err != nil {
				return nil, err
			}

			capabilities := allCapabilities.Clone()

			evtCapabilities, ok := capabilities[eventType]
			if !ok {
				continue
			}

			// try to convert the most efficient approver so that weak approvers are still backed by a discarder
			sort.Slice(evtCapabilities, func(i, j int) bool {
				return evtCapabilities[i].FilterWeight > evtCapabilities[j].FilterWeight
			})

			for _, evtCapability := range evtCapabilities {
				evtCapability.FilterMode = rules.ApproverOnlyMode

				filterReport, err = computeFilters(rs, capabilities)
				if err != nil {
					return nil, err
				}

				// revert the capability
				evtCapability.FilterMode = rules.DefaultMode

				if len(filterReport.DiscardersReport.Invalid) == 0 {
					return filterReport, nil
				}
			}
		}
	} else {
		return filterReport, nil
	}

	// no improvement, return the initial report
	filterReport, err = computeFilters(rs, allCapabilities)
	if err != nil {
		return nil, err
	}

	return filterReport, nil
}
