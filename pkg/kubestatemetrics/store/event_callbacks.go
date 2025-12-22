// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package store

import (
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StoreEventType represents the type of store event
//
//nolint:revive // StoreEventType is an established type name in this package
type StoreEventType string

const (
	// EventAdd represents an object creation event
	EventAdd StoreEventType = "add"
	// EventUpdate represents an object modification event
	EventUpdate StoreEventType = "update"
	// EventDelete represents an object deletion event
	EventDelete StoreEventType = "delete"
)

// StoreEventCallback is a function type for handling store events
//
//nolint:revive // StoreEventCallback is an established type name in this package
type StoreEventCallback func(eventType StoreEventType, resourceType, namespace, name string, obj interface{})

// ExtractNamespaceAndName extracts namespace and name from Kubernetes objects
func ExtractNamespaceAndName(obj interface{}) (string, string) {
	switch o := obj.(type) {
	// don't need to add resources manually but it's a bit faster if we do
	case *appsv1.Deployment:
		return o.Namespace, o.Name
	case *appsv1.ReplicaSet:
		return o.Namespace, o.Name
	case *appsv1.StatefulSet:
		return o.Namespace, o.Name
	case *appsv1.ControllerRevision:
		return o.Namespace, o.Name
	case metav1.Object:
		return o.GetNamespace(), o.GetName()
	}

	// Fallback: try to use reflection to get common fields
	// This covers most Kubernetes objects that have ObjectMeta
	if objMeta := getObjectMeta(obj); objMeta != nil {
		return objMeta.GetNamespace(), objMeta.GetName()
	}

	return "", ""
}

// getObjectMeta attempts to extract ObjectMeta from various Kubernetes object types
func getObjectMeta(obj interface{}) metav1.Object {
	switch o := obj.(type) {
	case metav1.Object:
		return o
	}
	return nil
}
