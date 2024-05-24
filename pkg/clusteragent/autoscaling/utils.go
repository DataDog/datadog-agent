// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/twmb/murmur3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// BuildObjectID returns a string that uniquely identifies an object
func BuildObjectID(ns, name string) string {
	if ns == "" {
		return name
	}

	return ns + "/" + name
}

// FromUnstructured converts an unstructured object into target type
func FromUnstructured(obj runtime.Object, structDest any) error {
	unstrObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("Could not cast Unstructured object: %v", obj)
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(unstrObj.UnstructuredContent(), structDest)
}

// ToUnstructured converts an object into an Unstructured
func ToUnstructured(structIn any) (*unstructured.Unstructured, error) {
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(structIn)
	if err != nil {
		return nil, fmt.Errorf("Unable to convert DatadogMetric %v: %w", structIn, err)
	}

	unstructOut := &unstructured.Unstructured{}
	unstructOut.SetUnstructuredContent(content)
	return unstructOut, nil
}

// ObjectHash returns a hash of the object
func ObjectHash(spec interface{}) (string, error) {
	b, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}

	hasher := murmur3.New128()
	_, err = hasher.Write(b)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
