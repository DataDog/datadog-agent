// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// NotListed are the resources that the fake cluster serves no object of, with the reason. Every other resource is
// served in each setup.
var NotListed = map[string]string{
	"v1/terminated-pods": "the collector only reports deleted pods: the deletion step reports the pods of the cluster",
}

// Object is an object of the fake cluster: the resource that collects it, and its current state.
type Object struct {
	// FullName is the full name of the collector of the object.
	FullName string
	Resource Resource
	// State is the object as the fake cluster serves it.
	State runtime.Object
	// Metadata reports whether the check sends a structured metadata model for the object.
	Metadata bool
	// Collected reports whether the check collects the object in the setup.
	Collected bool
	// APIResource is what API discovery reports for a custom resource.
	APIResource *metav1.APIResource
	// Copy reports whether Env.AddCopies added the object.
	Copy bool
}

func (o *Object) uid() string { return uidOf(o.State) }

func uidOf(object any) string {
	accessor, err := meta.Accessor(object)
	if err != nil {
		return ""
	}
	return string(accessor.GetUID())
}

// newObjects returns an object of the served version of each resource, each with its own name and UID, and with
// fields that must never be sent.
func newObjects(t *testing.T, setup Setup) []*Object {
	builtinFixtures := resourceFixtures()
	customFixtures, err := customResourceFixtures()
	require.NoError(t, err)

	var objects []*Object
	for _, resource := range Resources() {
		if _, skipped := NotListed[resource.FixtureName(resource.Versions[0])]; skipped {
			continue
		}
		fullName := resource.FixtureName(ServedVersion(resource, setup))
		o := &Object{FullName: fullName, Resource: resource}
		if resource.IsCustomResource() {
			fixture, found := customFixtures[fullName]
			require.True(t, found, "no fixture for %s: add one, or add the resource to NotListed with the reason", fullName)
			o.State, o.APIResource = fixture.Object.DeepCopy(), &fixture.APIResource
		} else {
			fixture, found := builtinFixtures[fullName]
			require.True(t, found, "no fixture for %s: add one, or add the resource to NotListed with the reason", fullName)
			o.State, o.Metadata = fixture.Object.DeepCopyObject(), fixture.MetadataType != nil
		}

		accessor, err := meta.Accessor(o.State)
		require.NoError(t, err)
		accessor.SetUID(types.UID(fullName))
		accessor.SetName(strings.NewReplacer("/", "-", ".", "-").Replace(fullName))
		if resource.NodeType.String() == "Namespace" {
			// The cluster collector reads the creation time of kube-system.
			accessor.SetName("kube-system")
			accessor.SetNamespace("")
		}
		accessor.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply}})
		o.Collected = isCollected(o, setup)
		objects = append(objects, o)
	}
	return objects
}

// isCollected reports whether the check collects the object in the setup.
func isCollected(o *Object, setup Setup) bool {
	switch {
	case setup.DefaultCollectors && !o.Resource.IsCustomResource() && !o.Resource.isEnabled():
		return false
	// Helm releases are collected with the ConfigMap collector only.
	case o.FullName == "v1/helm-releases-configmaps" && setup.DefaultCollectors:
		return false
	default:
		return !slices.Contains(setup.Undiscovered, o.FullName)
	}
}
