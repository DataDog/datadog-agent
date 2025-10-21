// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package generic

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// ContainerFilter defines an interface to exclude containers based on Metadata
type ContainerFilter interface {
	IsExcluded(container *workloadmeta.Container) bool
}

// FuncContainerFilter allows any function to be used as a ContainerFilter
type FuncContainerFilter func(container *workloadmeta.Container) bool

// IsExcluded returns if a container should be excluded or not
func (f FuncContainerFilter) IsExcluded(container *workloadmeta.Container) bool {
	return f(container)
}

// ANDContainerFilter implements a logical AND between given filters
type ANDContainerFilter struct {
	Filters []ContainerFilter
}

// IsExcluded returns if a container should be excluded or not
func (f ANDContainerFilter) IsExcluded(container *workloadmeta.Container) bool {
	for _, filter := range f.Filters {
		if filter.IsExcluded(container) {
			return true
		}
	}

	return false
}

// LegacyContainerFilter allows to use old containers.Filter within this new framework
type LegacyContainerFilter struct {
	ContainerFilter workloadfilter.FilterBundle
	Store           workloadmeta.Component
}

// IsExcluded returns if a container should be excluded or not
func (f LegacyContainerFilter) IsExcluded(container *workloadmeta.Container) bool {
	if f.ContainerFilter == nil {
		return false
	}

	pod, _ := f.Store.GetKubernetesPodForContainer(container.ID)

	filterableContainer := workloadmetafilter.CreateContainer(container, workloadmetafilter.CreatePod(pod))
	return f.ContainerFilter.IsExcluded(filterableContainer)
}

// RuntimeContainerFilter filters containers by runtime
type RuntimeContainerFilter struct {
	Runtime workloadmeta.ContainerRuntime
}

// IsExcluded returns if a container should be excluded or not
func (f RuntimeContainerFilter) IsExcluded(container *workloadmeta.Container) bool {
	return container.Runtime != f.Runtime
}
