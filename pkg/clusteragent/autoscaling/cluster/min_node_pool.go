// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	corev1 "k8s.io/api/core/v1"
)

type minNodePool struct {
	name                     string            `json:"name"`
	nodePoolHash             string            `json:"target_hash"` // TODO utilize once this is part of payload
	recommendedInstanceTypes []string          `json:"recommended_instance_types"`
	labels                   map[string]string `json:"labels"`
	taints                   []corev1.Taint    `json:"taints"`
}

type minNodeClass struct {
	Metadata minMetadata `json:"metadata"`
}

type minMetadata struct {
	Name string `json:"name"`
}
