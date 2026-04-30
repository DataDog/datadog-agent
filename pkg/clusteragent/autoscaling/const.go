// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import "k8s.io/apimachinery/pkg/runtime/schema"

// DatadogMetricGVR is the GroupVersionResource for datadoghq.com/v1alpha1 DatadogMetrics.
// Shared between the externalmetrics controller and the workload HPA-migration controller
// so both always reference the same resource definition.
var DatadogMetricGVR = schema.GroupVersionResource{
	Group:    "datadoghq.com",
	Version:  "v1alpha1",
	Resource: "datadogmetrics",
}
