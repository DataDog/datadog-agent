// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package registry

import (
	"fmt"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SampleResource is the resource that the fixtures of a group-wide custom resource entry serve.
const SampleResource = "samples"

// FixtureResource returns the resource that the fixtures of the entry serve.
func (r Resource) FixtureResource() string {
	if r.IsCustomResource() && r.Resource == "" {
		return SampleResource
	}
	return r.Resource
}

// FixtureName returns the collector full name of the fixture of the entry in the version.
func (r Resource) FixtureName(version string) string {
	return strings.TrimPrefix(r.Group+"/"+version+"/"+r.FixtureResource(), "/")
}

// CustomResourceFixture is a generated object of a custom resource entry, with the API resource that discovery reports
// for it.
type CustomResourceFixture struct {
	Object      *unstructured.Unstructured
	APIResource metav1.APIResource
}

// CustomResourceFixtures returns a generated object for each version of each custom resource entry of the registry, by
// collector full name. Custom resources are processed as generic objects, so a generated object covers the entry.
func CustomResourceFixtures() (map[string]CustomResourceFixture, error) {
	fixtures := map[string]CustomResourceFixture{}
	for _, resource := range Resources() {
		if !resource.IsCustomResource() {
			continue
		}
		kind, err := kindOf(resource.FixtureResource())
		if err != nil {
			return nil, err
		}
		for _, version := range resource.Versions {
			fixtures[resource.FixtureName(version)] = CustomResourceFixture{
				Object: &unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": schema.GroupVersion{Group: resource.Group, Version: version}.String(),
					"kind":       kind,
					"metadata": map[string]interface{}{
						"name": "name", "namespace": "default", "uid": "uid", "resourceVersion": "1",
						"annotations": map[string]interface{}{"kubectl.kubernetes.io/last-applied-configuration": "secret"},
					},
					"spec": map[string]interface{}{"replicas": int64(1)},
				}},
				APIResource: metav1.APIResource{Name: resource.FixtureResource(), Kind: kind, Namespaced: true, Verbs: []string{"list", "watch"}},
			}
		}
	}
	return fixtures, nil
}

// kindOf returns the kind of a generated custom resource: the singular of its resource name.
func kindOf(resource string) (string, error) {
	singular := strings.TrimSuffix(resource, "s")
	if base, found := strings.CutSuffix(resource, "ies"); found {
		singular = base + "y"
	} else if base, found := strings.CutSuffix(resource, "sses"); found {
		singular = base + "ss"
	}
	if singular == "" {
		return "", fmt.Errorf("no kind for the resource %q", resource)
	}
	return strings.ToUpper(singular[:1]) + singular[1:], nil
}

// customResourceOfKind maps the kind of each generated custom resource fixture to its resource.
var customResourceOfKind = sync.OnceValues(func() (map[schema.GroupVersionKind]schema.GroupVersionResource, error) {
	fixtures, err := CustomResourceFixtures()
	if err != nil {
		return nil, err
	}
	resources := make(map[schema.GroupVersionKind]schema.GroupVersionResource, len(fixtures))
	for _, fixture := range fixtures {
		gvk := fixture.Object.GroupVersionKind()
		resources[gvk] = gvk.GroupVersion().WithResource(fixture.APIResource.Name)
	}
	return resources, nil
})

// resourceOf returns the resource of a kind: the resource of a generated custom resource fixture, or the guessed one,
// which is wrong for kinds such as Gateway.
func resourceOf(gvk schema.GroupVersionKind) schema.GroupVersionResource {
	resources, err := customResourceOfKind()
	if err != nil {
		panic(fmt.Sprintf("cannot generate the custom resource fixtures: %v", err))
	}
	if gvr, found := resources[gvk]; found {
		return gvr
	}
	gvr, _ := meta.UnsafeGuessKindToResource(gvk)
	return gvr
}
