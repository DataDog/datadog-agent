// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

//// Covered by test/integration/util/kube_apiserver/events_test.go

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetPods() retrieves all the pods in the Kubernetes cluster across all namespaces.
func (c *APIClient) GetPods() ([]v1.Pod, error) {
	podList, err := c.Cl.CoreV1().Pods(metav1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return []v1.Pod{}, err
	}

	return podList.Items, nil
}
