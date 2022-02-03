// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator
// +build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func boolPtr(v bool) *bool {
	return &v
}

func int32Ptr(v int32) *int32 {
	return &v
}

func int64Ptr(v int64) *int64 {
	return &v
}

func strPtr(v string) *string {
	return &v
}

func getTemplateWithResourceRequirements() corev1.PodTemplateSpec {
	parseRequests := resource.MustParse("250M")
	parseLimits := resource.MustParse("550M")
	return corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "aContainer",
					Resources: corev1.ResourceRequirements{
						Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: parseLimits},
						Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: parseRequests},
					},
				},
			},
			InitContainers: []corev1.Container{
				{
					Name: "aContainer",
					Resources: corev1.ResourceRequirements{
						Limits:   map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: parseLimits},
						Requests: map[corev1.ResourceName]resource.Quantity{corev1.ResourceMemory: parseRequests},
					},
				},
			},
		},
	}
}

func getExpectedModelResourceRequirements() []*model.ResourceRequirements {
	parseRequests := resource.MustParse("250M")
	parseLimits := resource.MustParse("550M")
	return []*model.ResourceRequirements{
		{
			Limits:   map[string]int64{corev1.ResourceMemory.String(): parseLimits.Value()},
			Requests: map[string]int64{corev1.ResourceMemory.String(): parseRequests.Value()},
			Name:     "aContainer",
			Type:     model.ResourceRequirementsType_container,
		}, {
			Limits:   map[string]int64{corev1.ResourceMemory.String(): parseLimits.Value()},
			Requests: map[string]int64{corev1.ResourceMemory.String(): parseRequests.Value()},
			Name:     "aContainer",
			Type:     model.ResourceRequirementsType_initContainer,
		},
	}
}
