// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package otelinstrumentation

import (
	"fmt"

	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// UnstructuredIntoInstrumentation converts an unstructured object into an Instrumentation.
func UnstructuredIntoInstrumentation(obj interface{}, structDest *otelv1alpha1.Instrumentation) error {
	unstrObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("could not cast unstructured object: %v", obj)
	}
	return runtime.DefaultUnstructuredConverter.FromUnstructured(unstrObj.UnstructuredContent(), structDest)
}

// UnstructuredFromInstrumentation converts an Instrumentation object into an Unstructured.
func UnstructuredFromInstrumentation(structIn *otelv1alpha1.Instrumentation, unstructOut *unstructured.Unstructured) error {
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(structIn)
	if err != nil {
		return fmt.Errorf("unable to convert Instrumentation %v: %w", structIn, err)
	}
	unstructOut.SetUnstructuredContent(content)
	return nil
}

// InstrumentationFromObject converts a runtime object (typed, unstructured, or tombstone)
// into an Instrumentation.
func InstrumentationFromObject(obj interface{}) (*otelv1alpha1.Instrumentation, error) {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	if cr, ok := obj.(*otelv1alpha1.Instrumentation); ok {
		return cr.DeepCopy(), nil
	}

	if _, ok := obj.(*unstructured.Unstructured); !ok {
		return nil, fmt.Errorf("unexpected Instrumentation object type %T", obj)
	}

	cr := &otelv1alpha1.Instrumentation{}
	if err := UnstructuredIntoInstrumentation(obj, cr); err != nil {
		return nil, err
	}
	return cr, nil
}
