// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package program

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// LegacyFilterProgram implements the legacy filtering system using
// containers.Filter as the underlying filter implementation.
type LegacyFilterProgram struct {
	Name                 string
	Filter               *containers.Filter
	InitializationErrors []error
}

var _ FilterProgram = &LegacyFilterProgram{}

// Evaluate evaluates the filter program for a Result (Included, Excluded, or Unknown)
func (n LegacyFilterProgram) Evaluate(entity workloadfilter.Filterable) workloadfilter.Result {
	if n.Filter == nil {
		return workloadfilter.Unknown
	}
	annotations, name, image, namespace := getLegacyFilterValues(entity)
	return n.Filter.GetResult(annotations, name, image, namespace)
}

// GetInitializationErrors returns any errors that occurred during the creation/initialization of the program
func (n LegacyFilterProgram) GetInitializationErrors() []error {
	return n.InitializationErrors
}

func getLegacyFilterValues(entity workloadfilter.Filterable) (annotations map[string]string, name, image, namespace string) {
	switch o := entity.(type) {
	case *workloadfilter.Container:
		return o.GetAnnotations(), o.GetName(), o.GetImage().GetReference(), o.GetPod().GetNamespace()
	case *workloadfilter.Pod:
		return o.GetAnnotations(), "", "", o.GetNamespace()
	case *workloadfilter.KubeService:
		return o.GetAnnotations(), o.GetName(), "", o.GetNamespace()
	case *workloadfilter.KubeEndpoint:
		return o.GetAnnotations(), o.GetName(), "", o.GetNamespace()
	default:
		return nil, "", "", ""
	}

}
