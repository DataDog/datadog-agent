// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cel

package program

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// LegacyFilterProgram implements the legacy filtering system using containers.Filter as
// the underlying filter implementation. This should only be used when CEL based filtering
// is not available.
type LegacyFilterProgram struct {
	Name                 string
	Filter               *containers.Filter
	InitializationErrors []error
}

// Evaluate evaluates the filter program for a Result (Included, Excluded, or Unknown)
func (n LegacyFilterProgram) Evaluate(entity workloadfilter.Filterable) (workloadfilter.Result, []error) {
	if n.Filter == nil {
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

		isExcluded := n.Filter.IsExcluded(container.GetAnnotations(), container.GetName(), container.GetImage(), podNamespace)
		if isExcluded {
			return workloadfilter.Excluded, nil
		}
	case workloadfilter.PodType:
		pod := entity.(*workloadfilter.Pod)
		isExcluded := n.Filter.IsExcluded(pod.GetAnnotations(), "", "", pod.Namespace)
		if isExcluded {
			return workloadfilter.Excluded, nil
		}
	default:
		return workloadfilter.Unknown, nil
	}
	return workloadfilter.Unknown, nil
}

// GetInitializationErrors returns any errors that occurred during the creation/initialization of the program
func (n LegacyFilterProgram) GetInitializationErrors() []error {
	return n.InitializationErrors
}
