// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// KubeUnstructuredResource describes a Kubernetes Unstructured
// that implements the ReportResource interface
type KubeUnstructuredResource struct {
	unstructured.Unstructured
}

// NewKubeUnstructuredResource instantiates a new KubeUnstructuredResource
func NewKubeUnstructuredResource(obj unstructured.Unstructured) *KubeUnstructuredResource {
	return &KubeUnstructuredResource{
		Unstructured: obj,
	}
}

// ID returns the resource identifier
func (kr *KubeUnstructuredResource) ID() string {
	return string(kr.GetUID())
}

// Type returns the resource type
func (kr *KubeUnstructuredResource) Type() string {
	return "kube_" + strings.ToLower(kr.GetKind())
}

// BuildReportForUnstructured returns default Report for Kubernetes objects
func BuildReportForUnstructured(passed, aggregated bool, obj *KubeUnstructuredResource) *Report {
	gvk := obj.GroupVersionKind()
	return &Report{
		Passed:     passed,
		Aggregated: aggregated,
		Data: event.Data{
			KubeResourceFieldKind:      gvk.Kind,
			KubeResourceFieldGroup:     gvk.Group,
			KubeResourceFieldVersion:   gvk.Version,
			KubeResourceFieldName:      obj.GetName(),
			KubeResourceFieldNamespace: obj.GetNamespace(),
		},
		Resource: ReportResource{
			ID:   obj.ID(),
			Type: obj.Type(),
		},
	}
}

// BuildReportForError returns a report for the given error
func BuildReportForError(err error) *Report {
	return &Report{
		Passed:        false,
		Error:         err,
		CriticalError: true,
	}
}
