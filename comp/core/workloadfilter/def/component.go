// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadfilter provides the interface for the filter component
package workloadfilter

// team: container-platform

// Component is the component type.
//
// Filters have precedence based on their groupings.
// If a set of filters produces an Include or Exclude result, then subsequent sets will not be evaluated.
// Therefore, filters in lower-indexed groups will take precedence over those in higher-indexed groups.
type Component interface {
	// IsContainerExcluded returns true if the container is excluded by the selected container filter keys.
	IsContainerExcluded(container *Container, containerFilters [][]ContainerFilter) bool
	// IsPodExcluded returns true if the pod is excluded by the selected pod filter keys.
	IsPodExcluded(pod *Pod, podFilters [][]PodFilter) bool
	// IsServiceExcluded returns true if the service is excluded by the selected service filter keys.
	IsServiceExcluded(service *Service, serviceFilters [][]ServiceFilter) bool
	// IsEndpointExcluded returns true if the endpoint is excluded by the selected endpoint filter keys.
	IsEndpointExcluded(endpoint *Endpoint, endpointFilters [][]EndpointFilter) bool

	// GetContainerFilterInitializationErrors returns a list of errors
	// encountered during the initialization of the selected container filters.
	GetContainerFilterInitializationErrors(filters []ContainerFilter) []error

	// Get Autodiscovery filters
	GetContainerAutodiscoveryFilters(filterScope Scope) [][]ContainerFilter
	GetPodAutodiscoveryFilters(filterScope Scope) [][]PodFilter
	GetServiceAutodiscoveryFilters(filterScope Scope) [][]ServiceFilter
	GetEndpointAutodiscoveryFilters(filterScope Scope) [][]EndpointFilter

	// Get Shared Metric filters
	GetContainerSharedMetricFilters() [][]ContainerFilter
	GetPodSharedMetricFilters() [][]PodFilter

	// Get Container Specific filters
	GetContainerPausedFilters() [][]ContainerFilter
	GetContainerSBOMFilters() [][]ContainerFilter
}
