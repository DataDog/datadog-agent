// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

// Package mock provides a mock implementation of the Kubernetes discovery client.
// It is intended for use in unit tests that require a controlled Kubernetes API response.
package mock

import (
	openapiv2 "github.com/google/gnostic-models/openapiv2"
	"github.com/stretchr/testify/mock"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/openapi"
	"k8s.io/client-go/rest"
)

// DiscoveryClient is a mock implementation of the Kubernetes discovery client.
type DiscoveryClient struct {
	mock.Mock
}

// RESTClient returns nil as it's not used in the mock.
func (m *DiscoveryClient) RESTClient() rest.Interface {
	return nil
}

// ServerGroups returns nil as this method is not used in the mock.
func (m *DiscoveryClient) ServerGroups() (*v1.APIGroupList, error) {
	return nil, nil
}

// ServerResourcesForGroupVersion returns nil as this method is not used in the mock.
func (m *DiscoveryClient) ServerResourcesForGroupVersion(_ string) (*v1.APIResourceList, error) {
	return nil, nil
}

// ServerPreferredResources returns nil as this method is not used in the mock.
func (m *DiscoveryClient) ServerPreferredResources() ([]*v1.APIResourceList, error) {
	return nil, nil
}

// ServerPreferredNamespacedResources returns nil as this method is not used in the mock.
func (m *DiscoveryClient) ServerPreferredNamespacedResources() ([]*v1.APIResourceList, error) {
	return nil, nil
}

// ServerVersion returns nil as this method is not used in the mock.
func (m *DiscoveryClient) ServerVersion() (*version.Info, error) {
	return nil, nil
}

// OpenAPISchema returns nil as this method is not used in the mock.
func (m *DiscoveryClient) OpenAPISchema() (*openapiv2.Document, error) {
	return nil, nil
}

// OpenAPIV3 returns nil as this method is not used in the mock.
func (m *DiscoveryClient) OpenAPIV3() openapi.Client {
	return nil
}

// WithLegacy returns nil as this method is not used in the mock.
func (m *DiscoveryClient) WithLegacy() discovery.DiscoveryInterface {
	return nil
}

// ServerGroupsAndResources mocks the discovery API response for available API groups and resources.
func (m *DiscoveryClient) ServerGroupsAndResources() ([]*v1.APIGroup, []*v1.APIResourceList, error) {
	args := m.Called()
	return nil, args.Get(1).([]*v1.APIResourceList), args.Error(2)
}
