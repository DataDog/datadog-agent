// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
)

// Report contains the result of a compliance check
type Report struct {
	// Data contains arbitrary data linked to check evaluation
	Data event.Data
	// Resource associated with the report
	Resource ReportResource
	// Passed defines whether check was successful or not
	Passed bool
	// Aggregated defines whether check was aggregated or not
	Aggregated bool
	// Evaluator defines the eval engine that was used to generate this report
	Evaluator string
	// Error of the check evaluation
	Error error
	// UserProvidedError indicates if the error was provided by the user rule
	UserProvidedError bool
}

// ReportResource holds the id and type of the resource associated with a report
type ReportResource struct {
	ID   string
	Type string
}

// Reports aggregates compliance reports
type Reports []*Report

func isFailing(r *Report) bool {
	return !r.Passed && r.Error == nil
}

func (r Reports) Len() int           { return len(r) }
func (r Reports) Less(i, j int) bool { return isFailing(r[i]) && !isFailing(r[j]) }
func (r Reports) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
