// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/twmb/murmur3"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/dump"
)

// Semantic can do semantic deep equality checks for api objects.
// Customized from https://github.com/kubernetes/apimachinery/blob/master/pkg/api/equality/semantic.go
// - Change time comparison to remove sub-second precision as serialized time in k8s objects are truncated to seconds.
// Copyright 2014 The Kubernetes Authors.
var Semantic = conversion.EqualitiesOrDie(
	func(a, b resource.Quantity) bool {
		// Ignore formatting, only care that numeric value stayed the same.
		// TODO: if we decide it's important, it should be safe to start comparing the format.
		//
		// Uninitialized quantities are equivalent to 0 quantities.
		return a.Cmp(b) == 0
	},
	func(a, b metav1.MicroTime) bool {
		return a.Truncate(time.Second).UTC() == b.Truncate(time.Second).UTC()
	},
	func(a, b metav1.Time) bool {
		return a.Truncate(time.Second).UTC() == b.Truncate(time.Second).UTC()
	},
	func(a, b labels.Selector) bool {
		return a.String() == b.String()
	},
	func(a, b fields.Selector) bool {
		return a.String() == b.String()
	},
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
func ObjectHash(obj interface{}) (string, error) {
	hasher := murmur3.New64()
	_, err := hasher.Write([]byte(dump.ForHash(obj)))
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
