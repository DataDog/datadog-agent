// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package custommetrics

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
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	UID       string `json:"uid"`
}

type MetricsBundle struct {
	External []ExternalMetricValue
}
