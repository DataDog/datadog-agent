// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package ssi contains shared Single Step Instrumentation configuration types.
package ssi

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// WorkloadTarget identifies a Kubernetes workload targeted for instrumentation.
type WorkloadTarget struct {
	Kind      string
	Namespace string
	Name      string
}

// DDITarget is the per-target APM configuration extracted from a
// DatadogInstrumentation custom resource.
type DDITarget struct {
	CR             types.NamespacedName
	Enabled        bool
	TracerVersions map[string]string
	TracerConfigs  []corev1.EnvVar
}
