// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// PolicyReport describes the result of the kernel policy and the approvers for an event type
type PolicyReport struct {
	Mode      PolicyMode
	Flags     PolicyFlag
	Approvers rules.Approvers
}

// Report describes the event types and their associated policy reports
type Report struct {
	Policies map[string]*PolicyReport
}

// NewReport returns a new report
func NewReport() *Report {
	return &Report{
		Policies: make(map[string]*PolicyReport),
	}
}

// Reporter describes a reporter of policy application
type Reporter struct {
	report *Report
}

func (r *Reporter) getPolicyReport(eventType eval.EventType) *PolicyReport {
	if r.report.Policies[eventType] == nil {
		r.report.Policies[eventType] = &PolicyReport{Approvers: rules.Approvers{}}
	}
	return r.report.Policies[eventType]
}

// SetFilterPolicy is called when a passing policy for an event type is applied
func (r *Reporter) SetFilterPolicy(eventType eval.EventType, mode PolicyMode, flags PolicyFlag) error {
	policyReport := r.getPolicyReport(eventType)
	policyReport.Mode = mode
	policyReport.Flags = flags
	return nil
}

// SetApprovers is called when approvers are applied for an event type
func (r *Reporter) SetApprovers(eventType eval.EventType, approvers rules.Approvers) error {
	policyReport := r.getPolicyReport(eventType)
	policyReport.Approvers = approvers
	return nil
}

// GetReport returns the report
func (r *Reporter) GetReport() *Report {
	return r.report
}

// NewReporter instantiates a new reporter
func NewReporter() *Reporter {
	return &Reporter{report: NewReport()}
}
