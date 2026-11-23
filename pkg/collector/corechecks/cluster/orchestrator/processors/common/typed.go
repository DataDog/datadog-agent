// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/redact"
)

// TypedObject is a typed Kubernetes resource, such as *appsv1.Deployment.
type TypedObject[P any] interface {
	metav1.Object
	runtime.Object
	DeepCopy() P
}

// TypedHandlers implements the handlers that only depend on the object metadata of a typed resource. Handlers of a
// resource type embed it and implement the handlers that depend on the resource type.
type TypedHandlers[P TypedObject[P]] struct {
	BaseHandlers
}

// BeforeMarshalling sets the kind and the API version of the resource, which typed informers do not set.
func (TypedHandlers[P]) BeforeMarshalling(ctx processors.ProcessorContext, resource, _ interface{}) (skip bool) {
	resource.(P).GetObjectKind().SetGroupVersionKind(schema.FromAPIVersionAndKind(ctx.GetAPIVersion(), ctx.GetKind()))
	return
}

// ResourceList converts a typed list of resources to a list of interfaces.
func (TypedHandlers[P]) ResourceList(_ processors.ProcessorContext, list interface{}) []interface{} {
	resourceList := list.([]P)
	resources := make([]interface{}, 0, len(resourceList))
	for _, resource := range resourceList {
		resources = append(resources, resource)
	}
	return resources
}

// CloneResource returns a deep copy of the resource.
func (TypedHandlers[P]) CloneResource(resource interface{}) interface{} {
	return resource.(P).DeepCopy()
}

// ResourceVersionFromRaw returns the resource version of the raw resource.
func (TypedHandlers[P]) ResourceVersionFromRaw(_ processors.ProcessorContext, resource interface{}) string {
	return resource.(P).GetResourceVersion()
}

// ResourceUID returns the UID of the resource.
func (TypedHandlers[P]) ResourceUID(_ processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(P).GetUID()
}

// ResourceVersion returns the resource version of the resource.
func (TypedHandlers[P]) ResourceVersion(_ processors.ProcessorContext, resource, _ interface{}) string {
	return resource.(P).GetResourceVersion()
}

// ScrubBeforeExtraction removes sensitive annotations and labels from the resource.
func (TypedHandlers[P]) ScrubBeforeExtraction(_ processors.ProcessorContext, resource interface{}) {
	r := resource.(P)
	redact.RemoveSensitiveAnnotationsAndLabels(r.GetAnnotations(), r.GetLabels())
}
