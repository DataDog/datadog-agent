// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package v1 contains the types of the Cluster Agent API (v1).
package v1

import (
	"k8s.io/apimachinery/pkg/util/sets"
)

// NamespacesPodsStringsSet maps pod names to a set of strings
// keyed by the namespace a pod belongs to.
// This data structure allows for O(1) lookups of services given a
// namespace and pod name.
//
// The data is stored in the following schema:
//
//	{
//		"namespace1": {
//			"pod": { "svc1": {}, "svc2": {}, "svc3": {} ]
//		},
//	 "namespace2": {
//			"pod2": [ "svc1": {}, "svc2": {}, "svc3": {} ]
//		}
//	}
type NamespacesPodsStringsSet map[string]MapStringSet

// MapStringSet maps a set of string by a string key
type MapStringSet map[string]sets.Set[string]

/*
 TODO: we should replace the NamespacesPodsStringsSet struct by the following struct.
	   It may improves the API consistency.
type NamespacesPodsStringsSet struct {
	Namespaces map[string]PodsStringsSet `json:"namespaces"`
}

type PodsStringsSet struct {
	Pods map[string]sets.Set[string] `json:"pods"`
}
*/

// NewNamespacesPodsStringsSet return new initialized NamespacesPodsStringsSet instance
func NewNamespacesPodsStringsSet() NamespacesPodsStringsSet {
	panic("not called")
}

// DeepCopy used to copy NamespacesPodsStringsSet in another NamespacesPodsStringsSet
func (m NamespacesPodsStringsSet) DeepCopy(old *NamespacesPodsStringsSet) NamespacesPodsStringsSet {
	panic("not called")
}

// Get returns the list of strings for a given namespace and pod name.
func (m NamespacesPodsStringsSet) Get(namespace, podName string) ([]string, bool) {
	panic("not called")
}

// Set updates strings for a given namespace and pod name.
func (m NamespacesPodsStringsSet) Set(namespace, podName string, strings ...string) {
	panic("not called")
}

// Delete deletes strings for a given namespace.
func (m NamespacesPodsStringsSet) Delete(namespace string, strings ...string) {
	panic("not called")
}

// MetadataResponseBundle maps pod names to associated metadata.
type MetadataResponseBundle struct {
	// Services maps pod names to the names of the services targeting the pod.
	// keyed by the namespace a pod belongs to.
	Services NamespacesPodsStringsSet `json:"services,omitempty"`
}

// NewMetadataResponseBundle returns new MetadataResponseBundle initialized instance
func NewMetadataResponseBundle() *MetadataResponseBundle {
	panic("not called")
}

// MetadataResponse use to encore /api/v1/tags payloads
type MetadataResponse struct {
	Nodes    map[string]*MetadataResponseBundle `json:"Nodes,omitempty"`    // Nodes with uppercase for backward compatibility
	Warnings []string                           `json:"Warnings,omitempty"` // Warnings with uppercase for backward compatibility
	Errors   string                             `json:"Errors,omitempty"`   // Errors with uppercase for backward compatibility
	// TODO: Since it is Errors, it should be []string and not string
}

// NewMetadataResponse returns new NewMetadataResponse initialized instance
func NewMetadataResponse() *MetadataResponse {
	panic("not called")
}
