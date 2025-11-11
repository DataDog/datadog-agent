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
	// GetContainerFilters retrieves the selected container FilterBundle
	GetContainerFilters(containerFilters [][]ContainerFilter) FilterBundle
	// GetPodFilters retrieves the selected pod FilterBundle
	GetPodFilters(podFilters [][]PodFilter) FilterBundle
	// GetServiceFilters retrieves the selected service FilterBundle
	GetServiceFilters(serviceFilters [][]ServiceFilter) FilterBundle
	// GetEndpointFilters retrieves the selected endpoint FilterBundle
	GetEndpointFilters(endpointFilters [][]EndpointFilter) FilterBundle
	// GetProcessFilters retrieves the selected process FilterBundle
	GetProcessFilters(processFilters [][]ProcessFilter) FilterBundle

	// GetContainerAutodiscoveryFilters retrieves the container AD FilterBundle
	GetContainerAutodiscoveryFilters(filterScope Scope) FilterBundle
	// GetServiceAutodiscoveryFilters retrieves the service AD FilterBundle
	GetServiceAutodiscoveryFilters(filterScope Scope) FilterBundle
	// GetEndpointAutodiscoveryFilters retrieves the endpoint AD FilterBundle
	GetEndpointAutodiscoveryFilters(filterScope Scope) FilterBundle

	// GetContainerSharedMetricFilters retrieves the container shared metric FilterBundle
	GetContainerSharedMetricFilters() FilterBundle
	// GetContainerPausedFilters retrieves the container paused FilterBundle
	GetContainerPausedFilters() FilterBundle
	// GetPodSharedMetricFilters retrieves the pod shared metric FilterBundle
	GetPodSharedMetricFilters() FilterBundle

	// GetContainerSBOMFilters retrieves the container SBOM FilterBundle
	GetContainerSBOMFilters() FilterBundle
}
