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
	// Error of th check evaluation
	Error error
	// Describes if the error is critical
	CriticalError bool
}

// ReportResource holds the id and type of the resource associated with a report
type ReportResource struct {
	ID   string
	Type string
}
