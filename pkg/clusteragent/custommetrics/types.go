// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package custommetrics

type ExternalMetricValue struct {
	MetricName string            `json:"metricName"`
	Labels     map[string]string `json:"labels"`
	Timestamp  int64             `json:"ts"`
	HPARef     ObjectReference   `json:"hpaRef"`
	Value      int64             `json:"value"`
	Valid      bool              `json:"valid"`
}

type PodsMetricDescriptor struct {
	MetricName string          `json:"metricName"`
	HPARef     ObjectReference `json:"hpaRef"`
}

type ObjectMetricDescriptor struct {
	MetricName      string          `json:"metricName"`
	HPARef          ObjectReference `json:"hpaRef"`
	DescribedObject ObjectReference `json:"describedObject"`
}

// ObjectReference contains enough information to let you identify the referred resource.
type ObjectReference struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	APIVersion string `json:"apiVersion"`
}
