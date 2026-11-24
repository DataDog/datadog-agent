// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// Small objects for the tests that replace the default objects with WithObjects. The name of each object is its UID,
// so a test expects "live <name>" or "deleted <name>".

// ObjectMeta returns the metadata of an object of the default namespace.
func ObjectMeta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID(name), ResourceVersion: "1"}
}

// Deployment returns a deployment.
func Deployment(name string) *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: ObjectMeta(name)}
}

// AssignedPod returns a pod that is assigned to a node.
func AssignedPod(name string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: ObjectMeta(name), Spec: corev1.PodSpec{NodeName: "node"}}
}

// PendingPod returns a pod that no node runs yet.
func PendingPod(name string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: ObjectMeta(name)}
}

// Service returns a service.
func Service(name string) *corev1.Service {
	return &corev1.Service{ObjectMeta: ObjectMeta(name)}
}

// Node returns a node.
func Node(name string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(name), ResourceVersion: "1"}}
}

// Named returns a copy of the object with the name and UID.
func Named(t *testing.T, object runtime.Object, name string) runtime.Object {
	object = object.DeepCopyObject()
	accessor, err := meta.Accessor(object)
	require.NoError(t, err)
	accessor.SetName(name)
	accessor.SetUID(types.UID(name))
	return object
}
