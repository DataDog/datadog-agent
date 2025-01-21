// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types contains common Kubernetes types used by the Agent.
package types

// ObjectRelation represents a Kubernetes object's ObjectRelation.
type ObjectRelation struct {
	ParentGVRK GroupVersionResourceKind
	ParentName string

	// TODO: we can maybe cut down on having full GVRK?

	ChildGVRK GroupVersionResourceKind
	ChildName string
}

// GroupVersionResourceKind represents a Kubernetes object's Group, Version, Resource, and Kind.
type GroupVersionResourceKind struct {
	Group    string
	Version  string
	Resource string
	Kind     string
}

// GetAPIVersion returns the API version of the GroupVersionResourceKind.
func (g GroupVersionResourceKind) GetAPIVersion() string {
	return g.Group + "/" + g.Version
}
