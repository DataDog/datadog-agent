// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || kubeapiserver

// Package types contains common Kubernetes types used by the Agent.
package types

// ObjectRelation represents a Kubernetes object's ObjectRelation.
type ObjectRelation struct {
	ParentAPIVersion string
	ParentKind       string
	ParentName       string

	// TODO: we can maybe cut down on having full GVK?

	ChildAPIVersion string
	ChildKind       string
	ChildName       string
}

// TODO: investigate the impact of this import on agent images
// Maybe need to create a new type to avoid import of k8s.io/apimachinery/pkg/runtime/schema?

// // GroupVersionKind represents a Kubernetes object's Group, Version, and Kind.
// type GroupVersionKind struct {
// 	Group   string
// 	Version string
// 	Kind    string
// }

// // GetAPIVersion returns the API version of the GroupVersionKind.
// func (g GroupVersionKind) GetAPIVersion() string {
// 	return g.Group + "/" + g.Version
// }

// // GroupVersionResource represents a Kubernetes object's Group, Version, and Resource.
// type GroupVersionResource struct {
// 	Group    string
// 	Version  string
// 	Resource string
// }

// // GetAPIVersion returns the API version of the GroupVersionResource.
// func (g GroupVersionResource) GetAPIVersion() string {
// 	return g.Group + "/" + g.Version
// }
