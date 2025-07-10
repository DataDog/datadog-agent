// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package program contains the implementation of filtering programs.
package program

import (
	filterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

// FilterProgram is an interface that defines a method for evaluating a filter program.
type FilterProgram interface {

	// Evaluate evaluates the filter program for a Result (Included, Excluded, or Unknown)
	Evaluate(entity filterdef.Filterable) (filterdef.Result, []error)

	// GetInitializationErrors returns any errors that occurred
	// during the initialization of the filtering program
	GetInitializationErrors() []error
}
