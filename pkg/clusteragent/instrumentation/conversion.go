// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package instrumentation

import (
	"fmt"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// UnstructuredIntoDatadogInstrumentation converts an unstructured object into a DatadogInstrumentation.
func UnstructuredIntoDatadogInstrumentation(obj interface{}, structDest *datadoghq.DatadogInstrumentation) error {
	unstrObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("could not cast unstructured object: %v", obj)
	}
	return runtime.DefaultUnstructuredConverter.FromUnstructured(unstrObj.UnstructuredContent(), structDest)
}

// UnstructuredFromDatadogInstrumentation converts a DatadogInstrumentation object into an Unstructured.
func UnstructuredFromDatadogInstrumentation(structIn *datadoghq.DatadogInstrumentation, unstructOut *unstructured.Unstructured) error {
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(structIn)
	if err != nil {
		return fmt.Errorf("unable to convert DatadogInstrumentation %v: %w", structIn, err)
	}
	unstructOut.SetUnstructuredContent(content)
	return nil
}
