// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

// entityFilter defines an interface to include entity based on its attributes.
type entityFilter interface {
	IsIncluded(entity *Entity) bool
}

type namespaceFilter struct {
	namespace string
}

// IsIncluded returns true if the entity's namespace is included.
func (f *namespaceFilter) IsIncluded(entity *Entity) bool {
	return f.namespace == entity.Namespace
}

type loadNameFilter struct {
	loadName string
}

// IsIncluded returns true if the entity's load name is included.
func (f *loadNameFilter) IsIncluded(entity *Entity) bool {
	return f.loadName == entity.LoadName
}

type deploymentFilter struct {
	deployment string
}

// IsIncluded returns true if the entity's load name is included.
func (f *deploymentFilter) IsIncluded(entity *Entity) bool {
	return f.deployment == entity.Deployment
}

// ANDEntityFilter implements a logical AND between given filters
type ANDEntityFilter struct {
	Filters []entityFilter
}

// IsIncluded returns true if the entity is included by all filters.
func (f *ANDEntityFilter) IsIncluded(entity *Entity) bool {
	for _, filter := range f.Filters {
		if filter.IsIncluded(entity) {
			return true
		}
	}
	return false
}

func newANDEntityFilter(filters ...entityFilter) *ANDEntityFilter {
	f := &ANDEntityFilter{}
	for _, filter := range filters {
		f.Filters = append(f.Filters, filter)
	}
	return f
}
