// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package baseimpl contains the base implementation of the filter component.
package baseimpl

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
)

// filterBundle is the implementation of FilterBundle.
type filterBundle struct {
	log        log.Component
	filterSets [][]program.FilterProgram
}

func (fb *filterBundle) IsExcluded(obj workloadfilter.Filterable) bool {
	return fb.GetResult(obj) == workloadfilter.Excluded
}

func (fb *filterBundle) GetResult(obj workloadfilter.Filterable) workloadfilter.Result {
	for _, filterSet := range fb.filterSets {
		var setResult = workloadfilter.Unknown
		for _, prg := range filterSet {
			res := prg.Evaluate(obj)
			if res == workloadfilter.Included {
				fb.log.Debugf("Resource %s is included by filter %d", obj.Type(), prg)
				return res
			}
			if res == workloadfilter.Excluded {
				setResult = workloadfilter.Excluded
			}
		}
		// If the set of filters produces a Include/Exclude result,
		// then return the set's results and don't execute subsequent sets.
		if setResult != workloadfilter.Unknown {
			return setResult
		}
	}
	return workloadfilter.Unknown
}

func (fb *filterBundle) GetErrors() []error {
	var errs []error
	for _, filterSet := range fb.filterSets {
		for _, prg := range filterSet {
			if prgErrs := prg.GetInitializationErrors(); prgErrs != nil {
				errs = append(errs, prgErrs...)
			}
		}
	}
	return errs
}
