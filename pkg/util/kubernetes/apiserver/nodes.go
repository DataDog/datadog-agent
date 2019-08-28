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

// GetNodes() retrieves all the nodes in the Kubernetes cluster
func (c *APIClient) GetNodes() ([]v1.Node, error) {
	NodeList, err := c.Cl.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return []v1.Node{}, err
	}

	return NodeList.Items, nil
}
