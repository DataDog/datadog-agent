// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package mock provides a mock implementation of the Kubernetes discovery client.
// It is intended for use in unit tests that require a controlled Kubernetes API response.
//
// This package should only be used in test files and should not be imported into production code.
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

// MockDiscoveryClient is a mock implementation of the Kubernetes discovery client.
type MockDiscoveryClient struct {
	mock.Mock
}

// RESTClient returns nil as it's not used in the mock.
func (m *MockDiscoveryClient) RESTClient() rest.Interface {
	return nil
}

// ServerGroups returns nil as this method is not used in the mock.
func (m *MockDiscoveryClient) ServerGroups() (*v1.APIGroupList, error) {
	return nil, nil
}

// ServerResourcesForGroupVersion returns nil as this method is not used in the mock.
func (m *MockDiscoveryClient) ServerResourcesForGroupVersion(_ string) (*v1.APIResourceList, error) {
	return nil, nil
}

// ServerPreferredResources returns nil as this method is not used in the mock.
func (m *MockDiscoveryClient) ServerPreferredResources() ([]*v1.APIResourceList, error) {
	return nil, nil
}

// ServerPreferredNamespacedResources returns nil as this method is not used in the mock.
func (m *MockDiscoveryClient) ServerPreferredNamespacedResources() ([]*v1.APIResourceList, error) {
	return nil, nil
}

// ServerVersion returns nil as this method is not used in the mock.
func (m *MockDiscoveryClient) ServerVersion() (*version.Info, error) {
	return nil, nil
}

// OpenAPISchema returns nil as this method is not used in the mock.
func (m *MockDiscoveryClient) OpenAPISchema() (*openapiv2.Document, error) {
	return nil, nil
}

// OpenAPIV3 returns nil as this method is not used in the mock.
func (m *MockDiscoveryClient) OpenAPIV3() openapi.Client {
	return nil
}

// WithLegacy returns nil as this method is not used in the mock.
func (m *MockDiscoveryClient) WithLegacy() discovery.DiscoveryInterface {
	return nil
}

// ServerGroupsAndResources mocks the discovery API response for available API groups and resources.
func (m *MockDiscoveryClient) ServerGroupsAndResources() ([]*v1.APIGroup, []*v1.APIResourceList, error) {
	args := m.Called()
	return nil, args.Get(1).([]*v1.APIResourceList), args.Error(2)
}
