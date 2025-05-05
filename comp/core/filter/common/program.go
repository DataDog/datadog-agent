// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package commmon contains the implementation of common components.
package commmon

import (
	"fmt"

	"github.com/google/cel-go/cel"
)

// FilterResult is an enumeration that represents the possible results of a filter evaluation.
type FilterResult int

// filterResult represents the result of a filter evaluation.
const (
	Included FilterResult = iota
	Excluded
	Unknown
)

// FilterProgram is an interface that defines a method for evaluating a filter program.
type FilterProgram interface {
	IsExcluded(key string, val map[string]any) (FilterResult, error)
}

// InclExclProgram is a structure that holds two CEL programs: one for inclusion and one for exclusion.
type InclExclProgram struct {
	Include cel.Program
	Exclude cel.Program
}

// IsExcluded evaluates the filter program for inclusion and exclusion.
func (p InclExclProgram) IsExcluded(key string, val map[string]any) (FilterResult, error) {
	var lastError error

	if p.Include == nil && p.Exclude == nil {
		return Unknown, nil
	}

	if p.Include != nil {
		out, _, err := p.Include.Eval(map[string]any{key: val})
		if err == nil {
			res, ok := out.Value().(bool)
			if ok {
				if res {
					return Included, nil
				}
			} else {
				lastError = fmt.Errorf("error converting result to bool: %v", res)
			}
		} else {
			lastError = err
		}
	}

	if p.Exclude != nil {
		out, _, err := p.Exclude.Eval(map[string]any{key: val})
		if err == nil {
			res, ok := out.Value().(bool)
			if ok {
				if res {
					return Excluded, nil
				}
			} else {
				lastError = fmt.Errorf("error converting result to bool: %v", res)
			}
		} else {
			lastError = err
		}
	}

	return Unknown, lastError
}
