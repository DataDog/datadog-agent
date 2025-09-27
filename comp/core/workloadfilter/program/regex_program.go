// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package program

import (
	"regexp"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

// RegexProgram implements a regex-based filter program that is applied on a particular
// field of a workloadfilter.Filterable entity.
type RegexProgram struct {
	Name                 string
	ExcludeRegex         []*regexp.Regexp
	ExtractField         func(entity workloadfilter.Filterable) string
	InitializationErrors []error
}

var _ FilterProgram = &RegexProgram{}

// Evaluate evaluates the filter program for a Result (Included, Excluded, or Unknown)
func (p *RegexProgram) Evaluate(entity workloadfilter.Filterable) (workloadfilter.Result, []error) {
	if p.ExcludeRegex == nil {
		return workloadfilter.Unknown, nil
	}

	field := p.ExtractField(entity)
	for _, r := range p.ExcludeRegex {
		if r.MatchString(field) {
			return workloadfilter.Excluded, nil
		}
	}
	return workloadfilter.Unknown, nil
}

// GetInitializationErrors returns any errors that occurred during the creation/initialization of the program
func (p *RegexProgram) GetInitializationErrors() []error {
	return p.InitializationErrors
}
