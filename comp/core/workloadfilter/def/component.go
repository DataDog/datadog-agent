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
	IsContainerExcluded(container *Container, containerFilters [][]ContainerFilter) bool
	IsPodExcluded(pod *Pod, podFilters [][]PodFilter) bool
	IsServiceExcluded(service *Service, serviceFilters [][]ServiceFilter) bool
	IsEndpointExcluded(endpoint *Endpoint, endpointFilters [][]EndpointFilter) bool

	GetContainerFilterInitializationErrors(filter []ContainerFilter) []error
}
