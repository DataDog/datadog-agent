// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel

package program

import (
	"time"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/google/cel-go/cel"
)

// CELProgram is a structure that holds a CEL program for exclusion.
type CELProgram struct {
	Name                 string
	Exclude              cel.Program
	InitializationErrors []error
}

var _ FilterProgram = &CELProgram{}

var logLimiter = log.NewLogLimit(20, 10*time.Minute)

// Evaluate evaluates the filter program for a Result (Included, Excluded, or Unknown)
func (p CELProgram) Evaluate(entity workloadfilter.Filterable) workloadfilter.Result {
	if p.Exclude != nil {
		out, _, err := p.Exclude.Eval(map[string]any{string(entity.Type()): entity.Serialize()})
		if err == nil {
			res, ok := out.Value().(bool)
			if ok {
				if res {
					return workloadfilter.Excluded
				}
			} else {
				if logLimiter.ShouldLog() {
					log.Warnf(`filter '%s' from 'cel_workload_exclude' failed to convert value to bool: %v`, p.Name, out.Value())
				}
			}
		} else {
			log.Debugf(`filter '%s' from 'cel_workload_exclude' failed to evaluate: %v`, p.Name, err)
		}
	}

	return workloadfilter.Unknown
}

// GetInitializationErrors returns any errors that occurred during the creation/initialization of the program
func (p CELProgram) GetInitializationErrors() []error {
	return p.InitializationErrors
}
