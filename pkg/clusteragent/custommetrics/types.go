// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"k8s.io/apimachinery/pkg/types"
)

type ExternalMetricValue struct {
	OwnerRef   ObjectReference   `json:"ownerRef"`
	MetricName string            `json:"metricName"`
	Labels     map[string]string `json:"labels"`
	Timestamp  int64             `json:"ts"`
	Value      int64             `json:"value"`
	Valid      bool              `json:"valid"`
}

type ObjectReference struct {
	Kind       string    `json:"kind"`
	Namespace  string    `json:"namespace"`
	Name       string    `json:"name"`
	UID        types.UID `json:"uid"`
	APIVersion string    `json:"apiVersion"`
}
