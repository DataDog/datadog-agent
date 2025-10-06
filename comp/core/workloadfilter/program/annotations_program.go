// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package program

import (
	filterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// AnnotationsProgram is a structure that holds two sharable CEL programs:
// one for inclusion (higher priority) and one for exclusion (lower priority).
type AnnotationsProgram struct {
	Name          string
	ExcludePrefix string
}

var _ FilterProgram = &AnnotationsProgram{}

// Evaluate evaluates the filter program for a Result (Included, Excluded, or Unknown)
func (p AnnotationsProgram) Evaluate(entity filterdef.Filterable) (filterdef.Result, []error) {
	isExcluded := containers.IsExcludedByAnnotationInner(entity.GetAnnotations(), entity.GetName(), p.ExcludePrefix)
	if isExcluded {
		return filterdef.Excluded, nil
	}
	return filterdef.Unknown, nil
}

// GetInitializationErrors returns any errors that occurred during the creation/initialization of the program
func (p AnnotationsProgram) GetInitializationErrors() []error {
	return nil
}
