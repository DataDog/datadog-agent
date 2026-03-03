// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package transform holds api types transformation related files
package transform

import (
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// FromProtoToFilterReport transforms a proto to a kfilter filter report
func FromProtoToFilterReport(p *api.FilterReport) *kfilters.FilterReport {
	approverReports := make(map[eval.EventType]*kfilters.ApproverReport)

	toAcceptModeRules := func(r []*api.AcceptModeRule) []kfilters.AcceptModeRule {
		acceptModeRules := make([]kfilters.AcceptModeRule, len(r))
		for i, rule := range r {
			acceptModeRules[i] = kfilters.AcceptModeRule{
				RuleID: rule.RuleID,
			}
		}
		return acceptModeRules
	}

	for _, report := range p.GetApprovers() {
		approversToPrint := fromProtoToApprovers(report.GetApprovers())
		if len(approversToPrint) == 0 {
			approversToPrint = nil // This is here to ensure that the printed result is `"Approvers": null` and not `"Approvers": {}`
		}
		approverReports[report.EventType] = &kfilters.ApproverReport{
			Mode:            kfilters.PolicyMode(report.GetMode()),
			Approvers:       approversToPrint,
			AcceptModeRules: toAcceptModeRules(report.GetAcceptModeRules()),
		}
	}

	wholeReport := &kfilters.FilterReport{ApproverReports: approverReports}

	return wholeReport
}

// fromProtoToApprovers transforms a proto to a kfilter approvers
func fromProtoToApprovers(p *api.Approvers) rules.Approvers {
	approvers := make(rules.Approvers)

	for _, approver := range p.GetApproverDetails() {
		// The protobuf approver value is always a string, but the client approver value can be a string or an int
		var approverInterfaceVal interface{}
		approverVal := approver.GetValue()
		approverInt, err := strconv.Atoi(approver.GetValue())
		if err != nil {
			approverInterfaceVal = approverVal
		} else {
			approverInterfaceVal = approverInt
		}

		approvers[approver.GetField()] = append(approvers[approver.GetField()],
			rules.FilterValue{
				Field: approver.GetField(),
				Value: approverInterfaceVal,
				Type:  eval.FieldValueType(approver.GetType()),
			})
	}

	return approvers
}

// FromFilterReportToProtoRuleSetReportMessage returns a pointer to a RuleSetReportMessage
func FromFilterReportToProtoRuleSetReportMessage(filterReport *kfilters.FilterReport) *api.RuleSetReportMessage {
	var reports []*api.ApproverReport

	for key, report := range filterReport.ApproverReports {
		protoReport := &api.ApproverReport{
			EventType:       key,
			Mode:            uint32(report.Mode),
			Approvers:       fromApproversToProto(report.Approvers),
			AcceptModeRules: fromAcceptModeRulesToProto(report.AcceptModeRules),
		}

		reports = append(reports, protoReport)
	}

	return &api.RuleSetReportMessage{
		Filters: &api.FilterReport{
			Approvers: reports,
		},
	}
}

// fromAcceptModeRulesToProto transforms a kfilter to a proto accept mode rules
func fromAcceptModeRulesToProto(acceptModeRules []kfilters.AcceptModeRule) []*api.AcceptModeRule {
	protoAcceptModeRules := make([]*api.AcceptModeRule, len(acceptModeRules))
	for i, rule := range acceptModeRules {
		protoAcceptModeRules[i] = &api.AcceptModeRule{
			RuleID: rule.RuleID,
		}
	}
	return protoAcceptModeRules
}

// fromApproversToProto transforms a kfilter to a proto approvers
func fromApproversToProto(approvers rules.Approvers) *api.Approvers {
	protoApprovers := new(api.Approvers)

	for field, filterValues := range approvers {
		for _, filterValue := range filterValues {
			protoApprovers.Field = field
			stringFilterValue, ok := filterValue.Value.(string)
			if !ok {
				intFilterValue := filterValue.Value.(int)
				protoApprovers.ApproverDetails = append(protoApprovers.ApproverDetails, &api.ApproverDetails{
					Field: filterValue.Field,
					Value: strconv.Itoa(intFilterValue),
					Type:  int32(filterValue.Type),
				})
			} else {
				protoApprovers.ApproverDetails = append(protoApprovers.ApproverDetails, &api.ApproverDetails{
					Field: filterValue.Field,
					Value: stringFilterValue,
					Type:  int32(filterValue.Type),
				})
			}
		}
	}

	return protoApprovers
}
