// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containers provides container-related utilities.
// Filter types have moved to comp/core/workloadfilter/legacy.
package containers

import (
	legacy "github.com/DataDog/datadog-agent/comp/core/workloadfilter/legacy"
)

// FilterType indicates the container filter type.
//
// Deprecated: use comp/core/workloadfilter.Component instead.
type FilterType = legacy.FilterType

const (
	// GlobalFilter is used to cover both MetricsFilter and LogsFilter filter types.
	//
	// Deprecated: use comp/core/workloadfilter.Component instead.
	GlobalFilter FilterType = legacy.GlobalFilter
	// MetricsFilter refers to the Metrics filter type.
	//
	// Deprecated: use comp/core/workloadfilter.Component instead.
	MetricsFilter FilterType = legacy.MetricsFilter
	// LogsFilter refers to the Logs filter type.
	//
	// Deprecated: use comp/core/workloadfilter.Component instead.
	LogsFilter FilterType = legacy.LogsFilter

	// KubeNamespaceFilterPrefix is the prefix used for Kubernetes namespaces.
	//
	// Deprecated: use comp/core/workloadfilter.Component instead.
	KubeNamespaceFilterPrefix = legacy.KubeNamespaceFilterPrefix
)

// Filter holds the state for the container filtering logic.
//
// Deprecated: use comp/core/workloadfilter.Component instead.
type Filter = legacy.Filter

// NewFilter creates a new container filter from include and exclude pattern slices.
//
// Deprecated: use comp/core/workloadfilter.Component instead.
func NewFilter(ft FilterType, includeList, excludeList []string) (*Filter, error) {
	return legacy.NewFilter(ft, includeList, excludeList)
}

// GetPauseContainerExcludeList returns the exclude list for pause containers.
//
// Deprecated: use comp/core/workloadfilter.Component instead.
func GetPauseContainerExcludeList() []string {
	return legacy.GetPauseContainerExcludeList()
}

// GetPauseContainerFilter returns a filter that excludes pause containers when excludePause is true.
// The excludePause value should come from the "exclude_pause_container" config key.
//
// Deprecated: use comp/core/workloadfilter.Component instead.
func GetPauseContainerFilter(excludePause bool) (*Filter, error) {
	return legacy.GetPauseContainerFilter(excludePause)
}

// IsExcludedByAnnotationInner checks if an entity is excluded by annotations.
//
// Deprecated: use comp/core/workloadfilter.Component instead.
func IsExcludedByAnnotationInner(annotations map[string]string, containerName string, excludePrefix string) bool {
	return legacy.IsExcludedByAnnotationInner(annotations, containerName, excludePrefix)
}

// PreprocessImageFilter modifies image filters to handle strict matches without tags.
//
// Deprecated: use comp/core/workloadfilter.Component instead.
func PreprocessImageFilter(imageFilter string) string {
	return legacy.PreprocessImageFilter(imageFilter)
}
