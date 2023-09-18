// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package api holds api related files
package api

import (
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// FromProtoToKFiltersRuleSetReport transforms a proto to a kfilter rule set report
func (protoRuleSetReport *RuleSetReportMessage) FromProtoToKFiltersRuleSetReport() *kfilters.ApplyRuleSetReport {
	policies := make(map[eval.EventType]*kfilters.PolicyReport)

	for _, policy := range protoRuleSetReport.GetPolicies() {
		approversToPrint := *policy.GetApprovers().FromProtoToKFiltersApprovers()
		if len(approversToPrint) == 0 {
			approversToPrint = nil // This is here to ensure that the printed result is `"Approvers": null` and not `"Approvers": {}`
		}
		policies[policy.EventType] = &kfilters.PolicyReport{
			Mode:      kfilters.PolicyMode(policy.GetMode()),
			Flags:     kfilters.PolicyFlag(policy.GetFlags()),
			Approvers: approversToPrint,
		}
	}

	wholeReport := &kfilters.ApplyRuleSetReport{Policies: policies}

	return wholeReport
}

// FromProtoToKFiltersApprovers transforms a proto to a kfilter approvers
func (protoApprovers *Approvers) FromProtoToKFiltersApprovers() *rules.Approvers {
	approvers := make(rules.Approvers)

	for _, approver := range protoApprovers.GetApproverDetails() {
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

	return &approvers
}

// FromKFiltersToProtoRuleSetReport returns a pointer to a PolicyMessage
func FromKFiltersToProtoRuleSetReport(ruleSetReport *kfilters.ApplyRuleSetReport) *RuleSetReportMessage {
	var eventTypePolicy []*EventTypePolicy

	for key, policyReport := range ruleSetReport.Policies {
		detail := &EventTypePolicy{
			EventType: key,
			Mode:      uint32(policyReport.Mode),
			Flags:     uint32(policyReport.Flags),
			Approvers: FromKFiltersToProtoApprovers(policyReport.Approvers),
		}

		eventTypePolicy = append(eventTypePolicy, detail)
	}

	return &RuleSetReportMessage{
		Policies: eventTypePolicy,
	}
}

// FromKFiltersToProtoApprovers transforms a kfilter to a proto approvers
func FromKFiltersToProtoApprovers(approvers rules.Approvers) *Approvers {
	protoApprovers := new(Approvers)

	for field, filterValues := range approvers {
		for _, filterValue := range filterValues {
			protoApprovers.Field = field
			stringFilterValue, ok := filterValue.Value.(string)
			if !ok {
				intFilterValue := filterValue.Value.(int)
				protoApprovers.ApproverDetails = append(protoApprovers.ApproverDetails, &ApproverDetails{
					Field: filterValue.Field,
					Value: strconv.Itoa(intFilterValue),
					Type:  int32(filterValue.Type),
				})
			} else {
				protoApprovers.ApproverDetails = append(protoApprovers.ApproverDetails, &ApproverDetails{
					Field: filterValue.Field,
					Value: stringFilterValue,
					Type:  int32(filterValue.Type),
				})
			}
		}
	}

	return protoApprovers
}
