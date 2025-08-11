// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package program

import (
	"fmt"

	filterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"

	"github.com/google/cel-go/cel"
)

// CELProgram is a structure that holds two CEL programs:
// one for inclusion (higher priority) and one for exclusion (lower priority).
type CELProgram struct {
	Name                 string
	Include              cel.Program
	Exclude              cel.Program
	InitializationErrors []error
}

var _ FilterProgram = &CELProgram{}

// Evaluate evaluates the filter program for a Result (Included, Excluded, or Unknown)
func (p CELProgram) Evaluate(entity filterdef.Filterable) (filterdef.Result, []error) {
	var errs []error
	if p.Include != nil {
		out, _, err := p.Include.Eval(map[string]any{string(entity.Type()): entity.Serialize()})
		if err == nil {
			res, ok := out.Value().(bool)
			if ok {
				if res {
					return filterdef.Included, nil
				}
			} else {
				errs = append(errs, fmt.Errorf("include (%s) result not bool: %v", p.Name, out.Value()))
			}
		} else {
			errs = append(errs, fmt.Errorf("include (%s) eval error: %w", p.Name, err))
		}
	}

	if p.Exclude != nil {
		out, _, err := p.Exclude.Eval(map[string]any{string(entity.Type()): entity.Serialize()})
		if err == nil {
			res, ok := out.Value().(bool)
			if ok {
				if res {
					return filterdef.Excluded, nil
				}
			} else {
				errs = append(errs, fmt.Errorf("exclude (%s) result not bool: %v", p.Name, out.Value()))
			}
		} else {
			errs = append(errs, fmt.Errorf("exclude (%s) eval error: %w", p.Name, err))
		}
	}

	return filterdef.Unknown, errs
}

// GetInitializationErrors returns any errors that occurred during the creation/initialization of the program
func (p CELProgram) GetInitializationErrors() []error {
	return p.InitializationErrors
}
