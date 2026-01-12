// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package config

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// InjectionPattern is the main interface to implement to support a new proxy type
// for appsec injection. It is similar to the controller pattern used in
// controller-runtime, but much simpler.
// The controller watches for changes to a specific resource (Resource method)
// and calls the Added, Modified, Deleted methods accordingly.
// The IsInjectionPossible method is called at startup to determine if the pattern
// can be used in the current cluster (e.g. if the required CRDs are present).
// The Namespace method returns the namespace to watch, or metav1.NamespaceAll to watch all namespaces.
// The methods are called sequentially, there is no need to handle concurrency.
// The methods should return an error if something goes wrong, in which case the
// object will be re-queued with a backoff.
type InjectionPattern interface {
	// IsInjectionPossible returns true if the pattern can be used in the current cluster.
	IsInjectionPossible(ctx context.Context) error
	// Resource returns the GroupVersionResource to watch.
	Resource() schema.GroupVersionResource
	// Namespace returns the namespace to watch, or metav1.NamespaceAll to watch all namespaces.
	Namespace() string
	// Added is called when an object is added or at startup for existing objects. It should be idempotent.
	Added(ctx context.Context, obj *unstructured.Unstructured) error
	// Deleted is called when an object is deleted. It should be idempotent.
	Deleted(ctx context.Context, obj *unstructured.Unstructured) error
}
