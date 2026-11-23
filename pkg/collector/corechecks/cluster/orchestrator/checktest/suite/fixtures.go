// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/registry"
)

// UpdatedCopy returns a copy of an object with another resource version.
func UpdatedCopy(object runtime.Object, resourceVersion string) runtime.Object {
	updated := object.DeepCopyObject()
	accessor, _ := meta.Accessor(updated)
	accessor.SetResourceVersion(resourceVersion)
	return updated
}

// The fixtures are an object of each registry entry, and the expected objects are the fixtures that the check collects.

// NotListed are the registry entries that the fake cluster serves no object of, with the reason. Every other entry
// is served in each scenario.
var NotListed = map[string]string{
	"v1/terminated-pods": "the collector only reports deleted pods: the deletion step reports the pods of the cluster",
}

// ScenarioObject is an object of the fake cluster, with the registry entry that collects it.
type ScenarioObject struct {
	FullName string
	Object   runtime.Object
	Resource registry.Resource
	Metadata bool
	// APIResource is what API discovery reports for a custom resource.
	APIResource *metav1.APIResource
}

func (o ScenarioObject) UID() string {
	accessor, _ := meta.Accessor(o.Object)
	return string(accessor.GetUID())
}

// ServedVersion returns the version of the entry that the scenario serves.
func ServedVersion(resource registry.Resource, cfg ScenarioConfig) string {
	if !cfg.OlderVersions {
		return resource.Versions[0]
	}
	return olderVersions()[resource.Group+"/"+resource.FixtureResource()]
}

// olderVersions returns the last version of each entry, by group and resource, like a cluster with older operators.
// The check selects the version of a custom resource in its group, so an entry keeps its first version when another
// entry of the group serves it.
var olderVersions = sync.OnceValue(func() map[string]string {
	versions := map[string]string{}
	for _, resource := range registry.Resources() {
		versions[resource.Group+"/"+resource.FixtureResource()] = resource.Versions[len(resource.Versions)-1]
	}
	for changed := true; changed; {
		changed = false
		for _, resource := range registry.Resources() {
			key := resource.Group + "/" + resource.FixtureResource()
			if !resource.IsCustomResource() || versions[key] == resource.Versions[0] {
				continue
			}
			for _, other := range registry.Resources() {
				otherKey := other.Group + "/" + other.FixtureResource()
				if other.Group == resource.Group && otherKey != key && versions[otherKey] == resource.Versions[0] {
					versions[key], changed = resource.Versions[0], true
					break
				}
			}
		}
	}
	return versions
})

// ScenarioObjects returns a fixture of the served version of each registry entry, each with its own name and UID, and
// with fields that must never be sent.
func ScenarioObjects(t *testing.T, cfg ScenarioConfig) []ScenarioObject {
	builtinFixtures := registry.ResourceFixtures()
	customFixtures, err := registry.CustomResourceFixtures()
	require.NoError(t, err)

	var objects []ScenarioObject
	for _, resource := range registry.Resources() {
		if _, skipped := NotListed[resource.FixtureName(resource.Versions[0])]; skipped {
			continue
		}
		fullName := resource.FixtureName(ServedVersion(resource, cfg))
		o := ScenarioObject{FullName: fullName, Resource: resource}
		if resource.IsCustomResource() {
			fixture, found := customFixtures[fullName]
			require.True(t, found, "no fixture for %s", fullName)
			o.Object, o.APIResource = fixture.Object.DeepCopy(), &fixture.APIResource
		} else {
			fixture, found := builtinFixtures[fullName]
			require.True(t, found, "no fixture for %s", fullName)
			o.Object, o.Metadata = fixture.Object.DeepCopyObject(), fixture.MetadataType != nil
		}

		accessor, err := meta.Accessor(o.Object)
		require.NoError(t, err)
		accessor.SetUID(types.UID(fullName))
		accessor.SetName(strings.NewReplacer("/", "-", ".", "-").Replace(fullName))
		if resource.NodeType.String() == "Namespace" {
			// The cluster collector reads the creation time of kube-system.
			accessor.SetName("kube-system")
			accessor.SetNamespace("")
		}
		accessor.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply}})
		objects = append(objects, o)
	}
	return objects
}

// CustomResourceDiscovery returns what API discovery reports for the custom resources of the scenario. Discovery
// reports the built-in resources of the registry itself.
func CustomResourceDiscovery(objects []ScenarioObject, cfg ScenarioConfig) []metav1.APIResourceList {
	var lists []metav1.APIResourceList
	for _, o := range objects {
		if o.APIResource == nil || slices.Contains(cfg.Undiscovered, o.FullName) {
			continue
		}
		groupVersion := o.FullName[:strings.LastIndex(o.FullName, "/")]
		index := slices.IndexFunc(lists, func(l metav1.APIResourceList) bool { return l.GroupVersion == groupVersion })
		if index < 0 {
			lists = append(lists, metav1.APIResourceList{GroupVersion: groupVersion})
			index = len(lists) - 1
		}
		lists[index].APIResources = append(lists[index].APIResources, *o.APIResource)
	}
	return lists
}

// ExpectedResource is an object that the check sends, in its current state.
type ExpectedResource struct {
	FullName string
	Deletes  bool
	Object   runtime.Object
}

type ExpectedSet struct {
	Live map[string]ExpectedResource
}
