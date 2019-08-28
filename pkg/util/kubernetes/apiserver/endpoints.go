// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"errors"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dderrors "github.com/StackVista/stackstate-agent/pkg/errors"
)

// SearchTargetPerName returns the endpoint matching a given target name. It allows
// to retrieve a given pod's endpoint address from a service.
func SearchTargetPerName(endpoints *v1.Endpoints, targetName string) (v1.EndpointAddress, error) {
	if endpoints == nil {
		return v1.EndpointAddress{}, errors.New("nil endpoints object passed")
	}
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			if addr.TargetRef == nil {
				continue
			}
			if addr.TargetRef.Name == targetName {
				return addr, nil
			}
		}
	}
	return v1.EndpointAddress{}, dderrors.NewNotFound("target named " + targetName)
}

// GetEndpoints() retrieves all the endpoints in the Kubernetes cluster across all namespaces.
func (c *APIClient) GetEndpoints() ([]v1.Endpoints, error) {
	endpointList, err := c.Cl.CoreV1().Endpoints(metav1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return []v1.Endpoints{}, err
	}

	return endpointList.Items, nil
}
