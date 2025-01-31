// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
)

// Injector provides a common interface for building components capable of
// mutating pods so that individual webhooks can share injectors.
type Injector interface {
	// InjectPod will optionally inject a pod, returning true if injection occurs and an error if there is a problem.
	// This must match the MutateFunc signature.
	InjectPod(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error)
}
