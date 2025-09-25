// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package program

import (
	"regexp"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

// RegexProgram implements a regex-based filter program for processes
type RegexProgram struct {
	Name                 string
	ExcludeRegex         []*regexp.Regexp
	InitializationErrors []error
}

var _ FilterProgram = &RegexProgram{}

// Evaluate evaluates the filter program for a Result (Included, Excluded, or Unknown)
func (p *RegexProgram) Evaluate(entity workloadfilter.Filterable) (workloadfilter.Result, []error) {
	if p.ExcludeRegex == nil {
		return workloadfilter.Unknown, nil
	}

	switch entity.Type() {
	case workloadfilter.ProcessType:
		process := entity.(*workloadfilter.Process)
		for _, r := range p.ExcludeRegex {
			if r.MatchString(process.GetCmdline()) {
				return workloadfilter.Excluded, nil
			}
		}
	case workloadfilter.ContainerType:
		container := entity.(*workloadfilter.Container)
		for _, r := range p.ExcludeRegex {
			if r.MatchString(container.GetImage()) {
				return workloadfilter.Excluded, nil
			}
		}
	default:
		return workloadfilter.Unknown, nil
	}
	return workloadfilter.Unknown, nil
}

// GetInitializationErrors returns any errors that occurred during the creation/initialization of the program
func (p *RegexProgram) GetInitializationErrors() []error {
	return p.InitializationErrors
}
