// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filter provides the interface for the filter component
package filter

// Component is the component type.
type Component interface {
	IsContainerExcluded(container Container, containerFilters []ContainerFilter, defaultValue bool) (bool, error)
	IsPodExcluded(pod Pod, podFilters []PodFilter, defaultValue bool) (bool, error)
}
