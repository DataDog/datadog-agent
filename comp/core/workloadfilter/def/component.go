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
	// GetKubeServiceFilters retrieves the selected kube service FilterBundle
	GetKubeServiceFilters(serviceFilters [][]KubeServiceFilter) FilterBundle
	// GetKubeEndpointFilters retrieves the selected kube endpoint FilterBundle
	GetKubeEndpointFilters(endpointFilters [][]KubeEndpointFilter) FilterBundle
	// GetProcessFilters retrieves the selected process FilterBundle
	GetProcessFilters(processFilters [][]ProcessFilter) FilterBundle

	// GetContainerAutodiscoveryFilters retrieves the container AD FilterBundle
	GetContainerAutodiscoveryFilters(filterScope Scope) FilterBundle
	// GetKubeServiceAutodiscoveryFilters retrieves the kube service AD FilterBundle
	GetKubeServiceAutodiscoveryFilters(filterScope Scope) FilterBundle
	// GetKubeEndpointAutodiscoveryFilters retrieves the kube endpoint AD FilterBundle
	GetKubeEndpointAutodiscoveryFilters(filterScope Scope) FilterBundle

	// GetContainerPausedFilters retrieves the container paused FilterBundle
	GetContainerPausedFilters() FilterBundle
	// GetContainerSharedMetricFilters retrieves the container shared metric FilterBundle
	GetContainerSharedMetricFilters() FilterBundle
	// GetPodSharedMetricFilters retrieves the pod shared metric FilterBundle
	GetPodSharedMetricFilters() FilterBundle

	// GetContainerSBOMFilters retrieves the container SBOM FilterBundle
	GetContainerSBOMFilters() FilterBundle
	// GetContainerRuntimeSecurityFilters retrieves the container RuntimeSecurity FilterBundle
	GetContainerRuntimeSecurityFilters() FilterBundle
	// GetContainerComplianceFilters retrieves the container Compliance FilterBundle
	GetContainerComplianceFilters() FilterBundle

	// String returns a string representation of the workloadfilter configuration
	// If useColor is true, the output will include ANSI color codes.
	String(useColor bool) string

	// Evaluate evaluates a program for a given entity
	Evaluate(programName string, entity Filterable) (Result, error)
}
