// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import "k8s.io/apimachinery/pkg/types"

type ExternalMetricValue struct {
	MetricName string            `json:"metricName"`
	Labels     map[string]string `json:"labels"`
	Timestamp  int64             `json:"ts"`
	HPA        ObjectReference   `json:"hpa"`
	Value      int64             `json:"value"`
	Valid      bool              `json:"valid"`
}

// ObjectReference contains enough information to let you identify the referred resource.
type ObjectReference struct {
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	UID       types.UID `json:"uid"`
}

type PodsMetricDescriptor struct {
	MetricName string          `json:"metricName"`
	HPA        ObjectReference `json:"hpa"`
}

type ObjectMetricDescriptor struct {
	MetricName      string          `json:"metricName"`
	HPA             ObjectReference `json:"hpa"`
	DescribedObject ObjectReference `json:"describedObject"`
}
