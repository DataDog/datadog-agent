// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package custommetrics

type ExternalMetricValue struct {
	MetricName   string            `json:"metricName"`
	Labels       map[string]string `json:"labels"`
	Timestamp    int64             `json:"ts"`
	HPAName      string            `json:"hpa_name"`
	HPANamespace string            `json:"hpa_namespace"`
	Value        int64             `json:"value"`
	Valid        bool              `json:"valid"`
}

type ExternalMetricInfo struct {
	MetricName   string
	HPAName      string
	HPANamespace string
}
