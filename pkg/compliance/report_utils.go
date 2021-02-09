// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// BuildReportForUnstructured returns default Report for Kubernetes objects
func BuildReportForUnstructured(passed bool, obj unstructured.Unstructured) *Report {
	gvk := obj.GroupVersionKind()
	return &Report{
		Passed: passed,
		Data: event.Data{
			KubeResourceFieldKind:      gvk.Kind,
			KubeResourceFieldGroup:     gvk.Group,
			KubeResourceFieldVersion:   gvk.Version,
			KubeResourceFieldName:      obj.GetName(),
			KubeResourceFieldNamespace: obj.GetNamespace(),
		},
	}
}
