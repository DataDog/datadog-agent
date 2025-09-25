// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cel

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

type legacyFilterProgram struct {
	filter *containers.Filter
	errors []error
}

func (n legacyFilterProgram) Evaluate(entity workloadfilter.Filterable) (workloadfilter.Result, []error) {
	if n.filter == nil {
		return workloadfilter.Unknown, nil
	}

	switch entity.Type() {
	case workloadfilter.ContainerType:
		container := entity.(*workloadfilter.Container)
		pod, ok := container.Owner.(*workloadfilter.Pod)
		podNamespace := ""
		if ok && pod != nil {
			podNamespace = pod.Namespace
		}

		isExcluded := n.filter.IsExcluded(container.GetAnnotations(), container.GetName(), container.GetImage(), podNamespace)
		if isExcluded {
			return workloadfilter.Excluded, nil
		}
	case workloadfilter.PodType:
		pod := entity.(*workloadfilter.Pod)
		isExcluded := n.filter.IsExcluded(pod.GetAnnotations(), pod.GetName(), "", pod.Namespace)
		if isExcluded {
			return workloadfilter.Excluded, nil
		}
	default:
		return workloadfilter.Unknown, nil
	}
	return workloadfilter.Unknown, nil
}

func (n legacyFilterProgram) GetInitializationErrors() []error {
	return n.errors
}

// createFromOldFilters creates a filter program using a wrapper around the legacy filter system. This is used in place
// of CELProgram when cel isn't available (ie. Dogstatsd flavor).
func createFromOldFilters(_ string, include, exclude []string, _ workloadfilter.ResourceType, logger log.Component) program.FilterProgram {
	filter, err := containers.NewFilter(containers.GlobalFilter, include, exclude)
	var initErrors []error
	if err != nil {
		initErrors = append(initErrors, err)
		logger.Warnf("Failed to create filter: %v", err)
	}

	return legacyFilterProgram{
		filter: filter,
		errors: initErrors,
	}
}
